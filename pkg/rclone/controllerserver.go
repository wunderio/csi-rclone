package rclone

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Validate arguments
	name := req.GetName()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name must be provided")
	}

	// if err := cs.validateVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
	// 	return nil, status.Error(codes.InvalidArgument, err.Error())
	// }

	reqCapacity := req.GetCapacityRange().GetRequiredBytes()

	// Load default connection settings from secret
	secret, e := getSecret("rclone-secret")
	if e != nil {
		klog.Warningf("getting secret error: %s", e)
		return nil, e
	}

	remote, remotePath, flags, e := extractFlags(map[string]string{}, secret)
	if e != nil {
		klog.Warningf("storage parameter error: %s", e)
		return nil, e
	}

	// Create subdirectory under base-dir
	// TODO: revisit permissions
	path := remotePath + "/" + name
	e = rcloneCmd("mkdir", remote, path, flags)
	if e != nil {
		if strings.Contains(e.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		return nil, status.Error(codes.Internal, e.Error())
	}
	// Remove capacity setting when provisioner 1.4.0 is available with fix for
	// https://github.com/kubernetes-csi/external-provisioner/pull/271
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{
		CapacityBytes: reqCapacity,
		VolumeId:      remote + ":" + path,
	}}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// Validate arguments
	volId := req.GetVolumeId()
	if len(volId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeteleVolume must be provided volume id")
	}

	splitedVolId := strings.SplitN(volId, ":", 2)
	remote, remotePath := splitedVolId[0], splitedVolId[1]

	// Load default connection settings from secret
	secret, e := getSecret("rclone-secret")
	if e != nil {
		klog.Warningf("getting secret error: %s", e)
		return nil, e
	}

	_, _, flags, e := extractFlags(map[string]string{}, secret)
	if e != nil {
		klog.Warningf("storage parameter error: %s", e)
		return nil, e
	}

	e = rcloneCmd("rmdirs", remote, remotePath, flags)
	if e != nil {
		if strings.Contains(e.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		return nil, status.Error(codes.Internal, e.Error())
	}
	// Remove capacity setting when provisioner 1.4.0 is available with fix for
	// https://github.com/kubernetes-csi/external-provisioner/pull/271
	return &csi.DeleteVolumeResponse{}, nil
}

func (*controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ControllerExpandVolume not implemented")
}

func (cs *controllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{Volume: &csi.Volume{
		VolumeId: req.VolumeId,
	}}, nil
}

// single operand routine.
func rcloneCmd(cmd, remote, remotePath string, flags map[string]string) error {
	// rclone <operand> remote:path [flag]
	args := append(
		[]string{},
		cmd,
		fmt.Sprintf(":%s:%s", remote, remotePath),
	)

	// Add user supplied flags
	for k, v := range flags {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}

	klog.Infof("executing %s command cmd=rclone, remote=:%s:%s", cmd, remote, remotePath)

	out, err := exec.Command("rclone", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %v cmd: 'rclone' remote: ':%s:%s' output: %q",
			cmd, err, remote, remotePath, string(out))
	}

	return nil
}