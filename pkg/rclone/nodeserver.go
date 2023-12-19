// Node(Server) takes charge of volume mounting and unmounting.

package rclone

// Restructure this file !!!
// Follow lifecycle

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/SwissDataScienceCenter/csi-rclone/pkg/kube"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const CSI_ANNOTATION_PREFIX = "csi-rclone.dev/"

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter   *mount.SafeFormatAndMount
	RcloneOps Operations
}

// Mounting Volume (Preparation)
func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NodeStageVolume not implemented")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NodeUnstageVolume not implemented")
}

// Mounting Volume (Actual Mounting)
func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.Infof("NodePublishVolume: called with args %+v", *req)
	if err := validatePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	volumeId := req.GetVolumeId()
	volumeContext := req.GetVolumeContext()
	pvcName := volumeContext["pvcName"]
	pvcNamespace := volumeContext["pvcNamespace"]

	pvcSecret, err := GetPvcSecret(ctx, pvcNamespace, pvcName)
	if err != nil {
		return nil, err
	}
	remote, remotePath, configData, flags, e := extractFlags(req.GetVolumeContext(), req.GetSecrets(), pvcSecret)
	if e != nil {
		klog.Warningf("storage parameter error: %s", e)
		return nil, e
	}
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		// testing original mount point, make sure the mount link is valid
		if _, err := ioutil.ReadDir(targetPath); err == nil {
			klog.Infof("already mounted to target %s", targetPath)
			return &csi.NodePublishVolumeResponse{}, nil
		}
		// todo: mount link is invalid, now unmount and remount later (built-in functionality)
		klog.Warningf("ReadDir %s failed with %v, unmount this directory", targetPath, err)

		if err := ns.mounter.Unmount(targetPath); err != nil {
			klog.Errorf("Unmount directory %s failed with %v", targetPath, err)
			return nil, err
		}
	}

	rcloneVol := &RcloneVolume{
		ID:         volumeId,
		Remote:     remote,
		RemotePath: remotePath,
	}
	err = ns.RcloneOps.Mount(ctx, rcloneVol, targetPath, configData, flags)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = ns.WaitForMountAvailable(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func GetPvcSecret(ctx context.Context, pvcNamespace string, pvcName string) (*v1.Secret, error) {
	cs, err := kube.GetK8sClient()
	if pvcName == "" || pvcNamespace == "" {
		return nil, nil
	}
	pvc, err := cs.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var pvcSecret *v1.Secret
	if pvc != nil {
		annotations := pvc.GetAnnotations()
		if len(annotations) > 0 {
			if v, found := annotations[CSI_ANNOTATION_PREFIX+"secret-name"]; found {
				pvcSecret, err = cs.CoreV1().Secrets(pvcNamespace).Get(ctx, v, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
			}

		}
	}
	return pvcSecret, nil
}

func validatePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "empty volume id")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "empty target path")
	}

	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "no volume capability set")
	}
	return nil
}

func extractFlags(volumeContext map[string]string, secret map[string]string, pvcSecret *v1.Secret) (string, string, string, map[string]string, error) {

	// Empty argument list
	flags := make(map[string]string)

	// Secret values are default, gets merged and overriden by corresponding PV values
	if len(secret) > 0 {
		// Needs byte to string casting for map values
		for k, v := range secret {
			flags[k] = string(v)
		}
	} else {
		klog.Infof("No csi-rclone connection defaults secret found.")
	}

	if len(volumeContext) > 0 {
		for k, v := range volumeContext {
			if strings.HasPrefix(k, "storage.kubernetes.io/") {
				continue
			}
			flags[k] = v
		}
	}
	if pvcSecret != nil {
		if len(pvcSecret.StringData) > 0 {
			for k, v := range pvcSecret.StringData {
				flags[k] = v
			}
		}
	}

	if e := validateFlags(flags); e != nil {
		return "", "", "", flags, e
	}

	remote := flags["remote"]
	remotePath := flags["remotePath"]

	if remotePathSuffix, ok := flags["remotePathSuffix"]; ok {
		remotePath = remotePath + remotePathSuffix
		delete(flags, "remotePathSuffix")
	}

	configData, flags := extractConfigData(flags)

	return remote, remotePath, configData, flags, nil
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

func extractConfigData(parameters map[string]string) (string, map[string]string) {
	flags := make(map[string]string)
	for k, v := range parameters {
		flags[k] = v
	}
	var configData string
	var ok bool
	if configData, ok = flags["configData"]; ok {
		delete(flags, "configData")
	}

	delete(flags, "remote")
	delete(flags, "remotePath")

	return configData, flags
}

func (ns *nodeServer) WaitForMountAvailable(mountpoint string) error {
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			notMnt, _ := ns.mounter.IsLikelyNotMountPoint(mountpoint)
			if !notMnt {
				return nil
			}
		case <-time.After(1 * time.Minute):
			return errors.New("wait for mount available timeout")
		}
	}
}

// Unmounting Volumes
func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.Infof("NodeUnPublishVolume: called with args %+v", *req)
	if err := validateUnPublishVolumeRequest(req); err != nil {
		return nil, err
	}
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	if _, err := ns.RcloneOps.GetVolumeById(ctx, req.GetVolumeId()); err == ErrVolumeNotFound {
		mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err := ns.RcloneOps.Unmount(ctx, req.GetVolumeId()); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func validateUnPublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "empty volume id")
	}

	if req.GetTargetPath() == "" {
		return status.Error(codes.InvalidArgument, "empty target path")
	}

	return nil
}

// Resizing Volume
func (*nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NodeExpandVolume not implemented")
}
