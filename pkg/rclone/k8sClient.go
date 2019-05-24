package rclone

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var clientset *kubernetes.Clientset

func GetK8sClient() (*kubernetes.Clientset, error) {
	if clientset != nil {
		return clientset, nil
	}

	config, e := rest.InClusterConfig()
	if e != nil {
		return nil, e
	}

	clientset, e = kubernetes.NewForConfig(config)
	if e != nil {
		return nil, e
	}
	return clientset, nil
}
