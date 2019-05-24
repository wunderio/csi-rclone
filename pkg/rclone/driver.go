package rclone

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ns    *nodeServer
	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	driverName = "csi-rclone"
	Version   = "latest"
	BuildTime = "1970-01-01 00:00:00"
)

func NewDriver(nodeID, endpoint string) *driver {
	glog.Infof("Starting new %s driver in version %s built %s", driverName, Version, BuildTime)

	d := &driver{}

	d.endpoint = endpoint

	d.csiDriver = csicommon.NewCSIDriver(driverName, Version, nodeID)
	d.csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	d.csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_UNKNOWN})

	return d
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		mounts:            map[string]*mountPoint{},
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
