// The Controller(Server) is responsible for creating, deleting, attaching, and detaching volumes and snapshots.

package rclone

import (
	"os"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	RcloneOps      Operations
	active_volumes map[string]int64
	mutex          sync.RWMutex
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volId := req.GetVolumeId()
	if len(volId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities must be provided volume id")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities without capabilities")
	}

	cs.mutex.Lock()
	if _, ok := cs.active_volumes[volId]; !ok {
		cs.mutex.Unlock()
		return nil, status.Errorf(codes.NotFound, "Volume %s not found", volId)
	}
	cs.mutex.Unlock()
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
	klog.Infof("ControllerCreateVolume: called with args %+v", *req)
	volumeName := req.GetName()
	if len(volumeName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name must be provided")
	}

	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume without capabilities")
	}

	// we don't use the size as it makes no sense for rclone. but csi drivers should succeed if
	// called twice with the same capacity for the same volume and fail if called twice with
	// differing capacity, so we need to remember it
	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	cs.mutex.Lock()
	if val, ok := cs.active_volumes[volumeName]; ok && val != volSizeBytes {
		cs.mutex.Unlock()
		return nil, status.Errorf(codes.AlreadyExists, "Volume operation already exists for volume %s", volumeName)
	}
	cs.active_volumes[volumeName] = volSizeBytes
	cs.mutex.Unlock()

	pvcName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	ns := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	// NOTE: We need the PVC name and namespace when mounting the volume, not here
	// that is why they are passed to the VolumeContext
	pvcSecret, err := GetPvcSecret(ctx, ns, pvcName)
	if err != nil {
		return nil, err
	}
	remote, remotePath, _, _, err := extractFlags(req.GetParameters(), req.GetSecrets(), pvcSecret, nil)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: %v", err)
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId: volumeName,
			VolumeContext: map[string]string{
				"secretName": pvcName,
				"namespace":  ns,
				"remote":     remote,
				"remotePath": remotePath,
			},
		},
	}, nil

}

// Delete Volume
func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volId := req.GetVolumeId()
	if len(volId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeteleVolume must be provided volume id")
	}
	cs.mutex.Lock()
	delete(cs.active_volumes, volId)
	cs.mutex.Unlock()

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

func (cs *controllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return &csi.ControllerModifyVolumeResponse{}, nil
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
