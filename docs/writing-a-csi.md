# Writing a CSI

- Use the CSI Test suite: [CSI Sanity](https://kubernetes-csi.github.io/docs/testing-drivers.html)
- Use the [Sidecars](https://kubernetes-csi.github.io/docs/sidecar-containers.html) which make sense for your driver, but use them!
- All operations need to be idempotant
- Minimum setup contains 8 Methods (described below)
- `Controller` and `Node` can be combined in a single binary

## Phases

1. Startup (Identity)
    - `GetPluginCapabilities``
    - `Probe`` - Are you still alive?
2. Create (Controller)
    - `CreateVolume` - User has requested a volume (PVC) -> which needs to be created
    - `ControllerPublishVolume` - Attach the volume to the required worker (eg. using the openstack api)
3. Use (Node)
    - `NodeStageVolume` - Partition, formant and mount to a staging path
    - `NodePublishVolume` - Bind mount to the container's required location
4. Stop (Node)
    - `NodeUnpublishVolume` - Remove the bind mount from the container's folder
    - `NodeUnstageVolume` - Remove the regular mount for the device - at this point the PVC is updated and availabe for another pod to use
5. Cleanup (Controller)
    - `ControllerUnpublishVolume` - Detach the volume from the current node, but leave it availabe for use
    - `DeleteVolume` - Entirely delete the volume (eg. using the openstack api) (except the PV is set to `retain`?)
    
## Kubernetes Resource Lifecycle

- Create PVC + pod
    1. Controller Create volume
    2. Controller Publish volume
    3. Node Stage Volume
    4. Node Publish volume

- Delete Pod
    1. Node Unpublish volume
    2. Node Unstage Volume
    3. Controller Unpublish volume

- Delete PVC
    - Controller Delete volume
    
## Installation to the Cluster

- `ServiceAccount` - RBAC
- `CSIDriver` - Kubernetes resource
- `Controller` - Statefulset - Your Driver + sidecar
- `Node` - Daemonset - Your Driver + sidecar