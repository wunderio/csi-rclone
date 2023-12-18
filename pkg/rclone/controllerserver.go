// The Controller(Server) is responsible for creating, deleting, attaching, and detaching volumes and snapshots.

package rclone

import (
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	// "github.com/google/uuid"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// "k8s.io/klog"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	RcloneOps Operations
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeteleVolume must be provided volume id")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume without capabilities")
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.VolumeContext,
			VolumeCapabilities: req.VolumeCapabilities,
			Parameters:         req.Parameters,
		},
	}, nil
}

// Attaching Volume
func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ControllerPublishVolume not implemented")
}

// Detaching Volume
func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ControllerUnpublishVolume not implemented")
}

// Provisioning Volumes
func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volumeName := req.GetName()
	if len(volumeName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name must be provided")
	}

	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume without capabilities")
	}

	pvcName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	ns := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	// NOTE: We need the PVC name and namespace when mounting the volume, not here
	// that is why they are passed to the VolumeContext
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId: volumeName,
			VolumeContext: map[string]string{
				"pvcName":      pvcName,
				"pvcNamespace": ns,
			},
		},
	}, nil

}

// Delete Volume
func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeteleVolume must be provided volume id")
	}

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

func saveRcloneConf(configData string) (string, error) {
	rcloneConf, err := os.CreateTemp("", "rclone.conf")
	if err != nil {
		return "", err
	}

	if _, err = rcloneConf.Write([]byte(configData)); err != nil {
		return "", err
	}

	if err = rcloneConf.Close(); err != nil {
		return "", err
	}
	return rcloneConf.Name(), nil
}
