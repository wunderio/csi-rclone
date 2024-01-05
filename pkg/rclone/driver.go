package rclone

import (
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/utils/mount"

	utilexec "k8s.io/utils/exec"
)

type Driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ns        *nodeServer
	cap       []*csi.VolumeCapability_AccessMode
	cscap     []*csi.ControllerServiceCapability
	RcloneOps Operations
}

var (
	DriverName    = "csi-rclone"
	DriverVersion = "SwissDataScienceCenter"
)

func NewDriver(nodeID, endpoint string, kubeClient *kubernetes.Clientset) *Driver {
	klog.Infof("Starting new %s RcloneDriver in version %s", DriverName, DriverVersion)

	d := &Driver{}
	d.endpoint = endpoint
	d.RcloneOps = NewRclone(kubeClient)

	d.csiDriver = csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	d.csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
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
			Exec:      utilexec.New(),
		},
		RcloneOps: d.RcloneOps,
	}
}

func NewControllerServer(d *Driver) *controllerServer {
	return &controllerServer{
		// Creating and passing the NewDefaultControllerServer is useless and unecessary
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		RcloneOps:               d.RcloneOps,
		active_volumes:          map[string]int64{},
		mutex:                   sync.RWMutex{},
	}
}

func (d *Driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	defer d.RcloneOps.Cleanup()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		NewControllerServer(d),
		NewNodeServer(d))
	s.Wait()
}
