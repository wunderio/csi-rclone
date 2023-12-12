package rclone

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
)

type Driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ns        *nodeServer
	cap       []*csi.VolumeCapability_AccessMode
	cscap     []*csi.ControllerServiceCapability
	rcloneOps Operations
}

var (
	DriverName    = "csi-rclone"
	DriverVersion = "dario"
)

// TODO: check where its running, only enable NodeServer when run as daemonset
// and Controler when run in the statefulset
// make it based on a flag, it is easy
func NewDriver(nodeID, endpoint string, kubeClient *kubernetes.Clientset) *Driver {
	klog.Infof("Starting new %s RcloneDriver in version %s", DriverName, DriverVersion)

	d := &Driver{}
	d.endpoint = endpoint
	d.rcloneOps = NewRclone(kubeClient)

	d.csiDriver = csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	d.csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	d.csiDriver.AddControllerServiceCapabilities(
		[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		})

	return d
}

func NewNodeServer(d *Driver) *nodeServer {
	return &nodeServer{
		// Creating and passing the NewDefaultNodeServer is useless and unecessary
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		mounter: &mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      mount.NewOsExec(),
		},
		RcloneOps: d.rcloneOps,
	}
}

func NewControllerServer(d *Driver) *controllerServer {
	return &controllerServer{
		// Creating and passing the NewDefaultControllerServer is useless and unecessary
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		RcloneOps:               d.rcloneOps,
	}
}

func (d *Driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		NewControllerServer(d),
		NewNodeServer(d))
	s.Wait()
}
