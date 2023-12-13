package test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	"github.com/li-il-li/csi-rclone/pkg/kube"
	"github.com/li-il-li/csi-rclone/pkg/rclone"
)

func TestMyDriver(t *testing.T) {
	// Setup the full driver and its environment
	endpoint := "unix:///plugin/csi.sock"
	kubeClient, err := kube.GetK8sClient()
	if err != nil {
		panic(err)
	}
	driver := rclone.NewDriver("hostname", endpoint, kubeClient)
	go driver.Run()

	mntDir, err := ioutil.TempDir("/tmp/sanity/mount/", "mount")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(mntDir)
	//defer os.RemoveAll(mntDir)

	mntStageDir, err := ioutil.TempDir("/tmp/sanity/stage/", "stage")
	if err != nil {
		t.Fatal(err)
	}
	os.Getwd()
	os.RemoveAll(mntStageDir)
	//defer os.RemoveAll(mntStageDir)
	cfg := &sanity.Config{

		TargetPath:  mntDir,
		StagingPath: mntStageDir,
		Address:     endpoint,
		SecretsFile: "testdata/secrets.yaml",
		TestVolumeParameters: map[string]string{
			"remote": "minio",
			"path":   "pruebas",
			"csi.storage.k8s.io/provisioner-secret-name":             "rclone-secret",
			"csi.storage.k8s.io/provisioner-secret-namespace":        "csi-rclone",
			"csi.storage.k8s.io/controller-publish-secret-name":      "rclone-secret",
			"csi.storage.k8s.io/controller-publish-secret-namespace": "csi-rclone",
			//"csi.storage.k8s.io/node-publish-secret-name": "",
			//"csi.storage.k8s.io/node-publish-secret-namespace": "${pvc.namespace}",
		},
	}
	sanity.Test(t, cfg)
}
