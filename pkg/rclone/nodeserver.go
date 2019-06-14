package rclone

import (
	"fmt"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounts map[string]*mountPoint
}

type mountPoint struct {
	VolumeId     string
	MountPath    string
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, e := mount.New("").IsLikelyNotMountPoint(targetPath)
	if e != nil {
		if os.IsNotExist(e) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	// Load default connection settings from secret
	secret, e := getSecret("rclone-secret")

	remote, remotePath, flags, e := extractFlags(req.GetVolumeContext(), secret)
	if e != nil {
		glog.Infof("storage parameter error: %s", e)
		return nil, e
	}

	e = Mount(remote, remotePath, targetPath, flags)
	if e != nil {
		if os.IsPermission(e) {
			return nil, status.Error(codes.PermissionDenied, e.Error())
		}
		if strings.Contains(e.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		return nil, status.Error(codes.Internal, e.Error())
	}
	ns.mounts[req.VolumeId] = &mountPoint{MountPath: targetPath, VolumeId: req.VolumeId}

	return &csi.NodePublishVolumeResponse{}, nil
}

func extractFlags(volumeContext map[string]string, secret *v1.Secret) (string, string, map[string]string, error) {
	
	// Empty argument list
	flags := make(map[string]string)

	// Secret values are default, gets merged and overriden by corresponding PV values
	if secret.Data != nil && len(secret.Data) > 0 {

		// Needs byte to string casting for map values
		for k, v := range secret.Data {
		    flags[k] = string(v)
		}
	} else {
		glog.Infof("No csi-rclone connection defaults secret found.")
	}

	if len(volumeContext) > 0 {
		for k, v := range volumeContext {
		    flags[k] = v
		}
	}

	if e := validateFlags(flags); e != nil {
		return "", "", flags, e
	}

	remote := flags["remote"]
	remotePath := flags["remotePath"]

	if remotePathSuffix, ok := flags["remotePathSuffix"]; ok {
	    remotePath = remotePath + remotePathSuffix
	    delete(flags, "remotePathSuffix")
	}

	delete(flags, "remote")
	delete(flags, "remotePath")

	return remote, remotePath, flags, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = util.UnmountPath(req.GetTargetPath(), mount.New(""))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if point, ok := ns.mounts[req.VolumeId]; ok {
		delete(ns.mounts, point.VolumeId)
		glog.Infof("successfully unmount volume: %s", point)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func validateFlags(flags map[string]string) error {
	if _, ok := flags["remote"]; !ok {
		return status.Errorf(codes.InvalidArgument, "missing volume context value: remote")
	}
	if _, ok := flags["remotePath"]; !ok {
		return status.Errorf(codes.InvalidArgument, "missing volume context value: remotePath")
	}
	return nil
}

func getSecret(secretName string) (*v1.Secret, error) {
	clientset, e := GetK8sClient()
	if e != nil {
		return nil, status.Errorf(codes.Internal, "can not create kubernetes client: %s", e)
	}

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
        clientcmd.NewDefaultClientConfigLoadingRules(),
        &clientcmd.ConfigOverrides{},
	)

	namespace, _, err := kubeconfig.Namespace()
    if err != nil {
        return nil, status.Errorf(codes.Internal, "can't get current namespace, error %s", secretName, err)
    }

    glog.Infof("Loading csi-rclone connection defaults from secret %s/%s", namespace, secretName)

    secret, e := clientset.CoreV1().
		Secrets(namespace).
		Get(secretName, metav1.GetOptions{})

	if e != nil {
		return nil, status.Errorf(codes.Internal, "can't load csi-rclone settings from secret %s: %s", secretName, e)
	}

	return secret, nil
}

// func Mount(params mountParams, target string, opts ...string) error {
func Mount(remote string, remotePath string, targetPath string, flags map[string]string) error {
	mountCmd := "rclone"
	mountArgs := []string{}

	defaultFlags := map[string]string{}
	defaultFlags["cache-info-age"] = "72h"
	defaultFlags["cache-chunk-clean-interval"] = "15m"
	defaultFlags["dir-cache-time"] = "5s"
	defaultFlags["vfs-cache-mode"] = "writes"
	defaultFlags["allow-other"] = "true"

	// rclone mount remote:path /path/to/mountpoint [flags]

	mountArgs = append(
		mountArgs,
		"mount",
		fmt.Sprintf(":%s:%s", remote, remotePath),
		targetPath,
		"--daemon",
	)

	// Add default flags
	for k, v := range defaultFlags {
		// Exclude overriden flags
		if _, ok := flags[k]; !ok {
			mountArgs = append(mountArgs,fmt.Sprintf("--%s=%s", k, v))
    	}
	}	

	// Add user supplied flags
	for k, v := range flags {
		mountArgs = append(mountArgs,fmt.Sprintf("--%s=%s", k, v))
	}

	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(targetPath, 0750)
	if err != nil {
		return err
	}

	glog.Infof("executing mount command cmd=%s, args=%s", mountCmd, mountArgs)

	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}

	return nil
}
