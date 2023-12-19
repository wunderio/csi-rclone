package test

import (
	"os"
	"testing"

	"github.com/SwissDataScienceCenter/csi-rclone/pkg/kube"
	"github.com/SwissDataScienceCenter/csi-rclone/pkg/rclone"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
)

func TestMyDriver(t *testing.T) {
	// Setup the full driver and its environment
	endpoint := "unix:///tmp/plugin/csi.sock"
	kubeClient, err := kube.GetK8sClient()
	if err != nil {
		panic(err)
	}
	driver := rclone.NewDriver("hostname", endpoint, kubeClient)
	go driver.Run()
	err = os.MkdirAll("/tmp/sanity/mount/", 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll("/tmp/sanity/stage/", 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll("/tmp/plugin/", 0700)
	if err != nil {
		t.Fatal(err)
	}

	mntDir, err := os.MkdirTemp("/tmp/sanity/mount/", "mount")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(mntDir)
	//defer os.RemoveAll(mntDir)

	mntStageDir, err := os.MkdirTemp("/tmp/sanity/stage/", "stage")
	if err != nil {
		t.Fatal(err)
	}
	os.Getwd()
	os.RemoveAll(mntStageDir)
	//defer os.RemoveAll(mntStageDir)
	cfg := sanity.NewTestConfig()
	cfg.Address = endpoint

	cfg.TargetPath = mntDir
	cfg.StagingPath = mntStageDir
	cfg.Address = endpoint
	cfg.SecretsFile = "testdata/secrets.yaml"
	cfg.TestVolumeParameters = map[string]string{
		"remote":     "my-s3",
		"remotePath": "giab",
		"configData": `[my-s3]
				type=s3
				provider=AWS`,
		// "csi.storage.k8s.io/provisioner-secret-name":             "rclone-secret",
		// "csi.storage.k8s.io/provisioner-secret-namespace":        "csi-rclone",
		// "csi.storage.k8s.io/controller-publish-secret-name":      "rclone-secret",
		// "csi.storage.k8s.io/controller-publish-secret-namespace": "csi-rclone",
		//"csi.storage.k8s.io/node-publish-secret-name": "",
		//"csi.storage.k8s.io/node-publish-secret-namespace": "${pvc.namespace}",
	}
	sanity.Test(t, cfg)
}
