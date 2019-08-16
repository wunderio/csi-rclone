package rclone

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/klog"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ns    *nodeServer
	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	DriverName    = "csi-rclone"
	DriverVersion = "latest"
)

func NewDriver(nodeID, endpoint string) *driver {
	klog.Infof("Starting new %s driver in version %s", DriverName, DriverVersion)

	d := &driver{}

	d.endpoint = endpoint

	d.csiDriver = csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	d.csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	d.csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_UNKNOWN})

	return d
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
	}
}

func (d *driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		csicommon.NewDefaultControllerServer(d.csiDriver),
		NewNodeServer(d))
	s.Wait()
}
