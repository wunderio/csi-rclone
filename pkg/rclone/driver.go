package rclone

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type Driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ns *nodeServer
	cs *controllerServer
}

var (
	DriverName    = "csi-rclone"
	DriverVersion = "latest"
)

func NewDriver(nodeID, endpoint string) *Driver {
	glog.Infof("Starting new %s driver in version %s", DriverName, DriverVersion)

	d := &Driver{}

	d.endpoint = endpoint

	d.csiDriver = csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	d.csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	d.csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME})

	d.cs = NewControllerServer(d)
	d.ns = NewNodeServer(d)

	return d
}

func NewNodeServer(d *Driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
	}
}

func NewControllerServer(d *Driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
	}
}

func (d *Driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		d.cs,
		d.ns,
	)
	s.Wait()
}
