

## Kubernetes cluster compatability
kubernetes 1.13.x

## Installing CSI driver to kubernetes cluster
TLDR: ` kubectl apply -f deploy/kubernetes --username=admin --password=123`

1. Set up storage backend. You can use [Minio](https://min.io/), Amazon S3 compatible cloud storage service.

2. Configure defaults by pushing secret to kube-system namespace. This is optional if you will always define `volumeAttributes` in PersistentVolume.

```
apiVersion: v1
kind: Secret
metadata:
  name: rclone-secret
type: Opaque
stringData:
  remote: "s3"
  remotePath: "projectname"
  s3-provider: "Minio"
  s3-endpoint: "http://minio-release.default:9000"
  s3-access-key-id: "ACCESS_KEY_ID"
  s3-secret-access-key: "SECRET_ACCESS_KEY"
```

Deploy example secret
> `kubectl apply -f example/kubernetes/rclone-secret-example.yaml --namespace kube-system`

3. You can override configuration via PersistentStorage resource definition. Leave volumeAttributes empty if you don't want to.

```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: data-rclone-example
  labels:
    name: data-rclone-example
spec:
  accessModes:
  - ReadWriteMany
  capacity:
    storage: 10Gi
  storageClassName: rclone
  csi:
    driver: csi-rclone
    volumeHandle: data-id
    volumeAttributes:
      remote: "s3"
      remotePath: "projectname/pvname"
      s3-provider: "Minio"
      s3-endpoint: "http://minio-release.default:9000"
      s3-access-key-id: "ACCESS_KEY_ID"
      s3-secret-access-key: "SECRET_ACCESS_KEY"
```

Deploy example definition
> `kubectl apply -f example/kubernetes/nginx-example.yaml`


## Building plugin and creating image
Current code is referencing projects repository on github.com. If you fork the repository, you have to change go includes in several places (use search and replace).


1. First push the changed code to remote. The build will use paths from `pkg/` directory.

2. Build the plugin
```
make plugin
```

3. Build the container and inject the plugin into it.
```
make container
```

4. Change docker.io account in `Makefile` and use `make push` to push the image to remote. 
``` 
make push
```

## TODO

+ ~Default settings from global configmap/secret~
+ ~Settings override from pvc~
+ ~Quick install overview + example deployment + svc for example~
+ ~Build instructions with the description how the go references remote repository~
- Helm chart with storageclass name overrride?
- Multiple / fixed versions?
- Terraform deployment / helm chart
- volumeAttributes sanitization?
- Controller & ControllerUnpublishVolume implementation. Delete remotePath (or bucket, if the remotePath is empty) when `reclaimPolicy` is set to `delete`.