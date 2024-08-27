package rclone

import (
	"regexp"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
}

type pvcMetadata struct {
	data        map[string]string
	labels      map[string]string
	annotations map[string]string
}

// source: https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner/blob/master/cmd/nfs-subdir-external-provisioner/provisioner.go
var pattern = regexp.MustCompile(`\${\.PVC\.((labels|annotations)\.(.*?)|.*?)}`)

// source: https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner/blob/master/cmd/nfs-subdir-external-provisioner/provisioner.go
func (meta *pvcMetadata) stringParser(str string) string {
	result := pattern.FindAllStringSubmatch(str, -1)
	for _, r := range result {
		switch r[2] {
		case "labels":
			str = strings.ReplaceAll(str, r[0], meta.labels[r[3]])
		case "annotations":
			str = strings.ReplaceAll(str, r[0], meta.annotations[r[3]])
		default:
			str = strings.ReplaceAll(str, r[0], meta.data[r[1]])
		}
	}

	return str
}

func (cs *controllerServer) getPVC(name, namespace string) (*v1.PersistentVolumeClaim, error) {
	clientset, e := GetK8sClient()
	if e != nil {
		return nil, status.Errorf(codes.Internal, "can not create kubernetes client: %s", e)
	}

	// Get the PVC
	pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Failed to get PVC: %v", err)
		return nil, err
	}

	return pvc, nil
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Parse the request to get the volume name, size, and parameters.
	volumeName := req.GetName()
	capacityBytes := req.GetCapacityRange().GetRequiredBytes()

	// Extract parameters from the request
	parameters := req.GetParameters()

	volumeContext := map[string]string{}

	pvcName := ""
	pvcNamespace := ""

	// parameter provided by external-provisioner (csi-provisioner)
	if val, ok := parameters["csi.storage.k8s.io/pvc/name"]; ok {
		pvcName = val
	}

	// parameter provided by external-provisioner (csi-provisioner)
	if val, ok := parameters["csi.storage.k8s.io/pvc/namespace"]; ok {
		pvcNamespace = val
	}

	// If PVC name is provided, load the PVC definition
	if pvcName != "" {

		pvc, err := cs.getPVC(pvcName, pvcNamespace)
		if err != nil {
			glog.Errorf("Failed to get PVC %s in namespace %s: %v", pvcName, pvcNamespace, err)
			return nil, err
		}

		// Extract PVC metadata
		metadata := &pvcMetadata{
			data: map[string]string{
				"name":      pvcName,
				"namespace": pvcNamespace,
			},
			labels:      pvc.Labels,
			annotations: pvc.Annotations,
		}

		if pathPattern, ok := parameters["pathPattern"]; ok {
			if pathPattern != "" {
				remotePathSuffix := metadata.stringParser(pathPattern)
				if remotePathSuffix != "" {
					if !strings.HasPrefix(remotePathSuffix, "/") {
						remotePathSuffix = "/" + remotePathSuffix
					}
					volumeContext["remotePathSuffix"] = remotePathSuffix
				}
			}
		}

		// if Annotation starts with "csi-rclone/", extract the key and value from the annotation
		for key, value := range metadata.annotations {
			if strings.HasPrefix(key, "csi-rclone/") {
				key = strings.TrimPrefix(key, "csi-rclone/")

				// Only allow some keys (umask, uid) to be passed to the volume context to avoid security issues
				if key == "umask" {
					volumeContext[key] = value
				}
			}
		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeName,
			CapacityBytes: capacityBytes,
			VolumeContext: volumeContext,
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	return &csi.DeleteVolumeResponse{}, nil
}
