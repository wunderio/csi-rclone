# CSI Design

On user request a secret and a PVC is created:

```yaml
# Secret
apiVersion: v1
kind: Secret
metadata:
  name: user1-secret
type: Opaque
data:
  username: user1_encrypted
  password: password1_encrypted
---
# PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: user1-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: my-storage-class    # Replace with your storage classname
  volumeMode: Filesystem
  dataSource: null
  selector: null
  volumeName: my-volume
  volumeAttributes:
    secret: <namespace>/<name>
    args:
    - "--flag1=baba"

```

Kubernetes does not support `volumeAttributes` for secrets in PVC by default.
Make sure the CSI plugin can handle this type of request and correctly extracts the secret
from the `volumeAttributes`.

Also add a section to the volume attributes that parses the flags.

There is also `controllerPublishSecretRef` and `nodePublishSecretRef`.

> The Kubernetes CSI development team also provides a GO lang package called protosanitizer that CSI driver developers may be used to remove values for all fields in a gRPC messages decorated with csi_secret. The library can be found in kubernetes-csi/csi-lib-utils/protosanitizer. The Kubernetes CSI Sidecar Containers and sample drivers use this library to ensure no sensitive information is logged.

I guess it should be the `controllerPublishSecretRef` because `ControllerPublish` functions attaches the volume to the node. Which in the rclone case would probably make sense.

> controllerPublishSecretRef: A reference to the secret object containing sensitive information to pass to the CSI driver to complete the CSI ControllerPublishVolume and ControllerUnpublishVolume calls. This field is optional, and may be empty if no secret is required. If the Secret contains more than one secret, all secrets are passed.

There is also a specific `readonly` field:

>readOnly: An optional boolean value indicating whether the volume is to be "ControllerPublished" (attached) as read only. Default is false. This value is passed to the CSI driver via the readonly field in the ControllerPublishVolumeRequest.

## Questions

1. Can I restrict the size of a volume mount?
2. Can I first mount a volume from switch or azure and then do the rclone stuff ontop (would solve q1)?