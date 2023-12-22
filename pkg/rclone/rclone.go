package rclone

import (
	"errors"
	"fmt"
	"os"
	os_exec "os/exec"

	"strings"
	"time"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/client/conditions"
	"k8s.io/utils/exec"
)

var (
	ErrVolumeNotFound = errors.New("volume is not found")
)

type Operations interface {
	CreateVol(ctx context.Context, volumeName, remote, remotePath, rcloneConfigPath string, pameters map[string]string) error
	DeleteVol(ctx context.Context, rcloneVolume *RcloneVolume, rcloneConfigPath string, pameters map[string]string) error
	Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath string, namespace string, rcloneConfigData string, pameters map[string]string) error
	Unmount(ctx context.Context, volumeId string, namespace string) error
	CleanupMountPoint(ctx context.Context, secrets, pameters map[string]string) error
	GetVolumeById(ctx context.Context, volumeId string) (*RcloneVolume, error)
}

type Rclone struct {
	execute    exec.Interface
	kubeClient *kubernetes.Clientset
}

type RcloneVolume struct {
	Remote     string
	RemotePath string
	ID         string
}

func (r *Rclone) Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath, namespace string, rcloneConfigData string, parameters map[string]string) error {
	mountCmd := "rclone"
	mountArgs := []string{}
	mountArgs = append(mountArgs, "mount")
	remoteWithPath := fmt.Sprintf("%s:%s", rcloneVolume.Remote, rcloneVolume.RemotePath)
	mountArgs = append(mountArgs, remoteWithPath)
	mountArgs = append(mountArgs, targetPath)
	mountArgs = append(mountArgs, "--daemon")
	defaultFlags := map[string]string{}
	defaultFlags["cache-info-age"] = "72h"
	defaultFlags["cache-chunk-clean-interval"] = "15m"
	defaultFlags["dir-cache-time"] = "60s"
	defaultFlags["vfs-cache-mode"] = "off"

	defaultFlags["allow-other"] = "true"
	defaultFlags["allow-non-empty"] = "true"
	// Add default flags
	for k, v := range defaultFlags {
		// Exclude overriden flags
		if _, ok := parameters[k]; !ok {
			if v != "" {
				mountArgs = append(mountArgs, fmt.Sprintf("--%s=%s", k, v))
			} else {
				mountArgs = append(mountArgs, fmt.Sprintf("--%s", k))
			}
		}
	}

	// Add user supplied flags
	for k, v := range parameters {
		if v != "" {
			mountArgs = append(mountArgs, fmt.Sprintf("--%s=%s", k, v))
		} else {
			mountArgs = append(mountArgs, fmt.Sprintf("--%s", k))
		}
	}

	if rcloneConfigData != "" {

		configFile, err := os.CreateTemp("", "rclone.conf")
		if err != nil {
			return err
		}

		// Normally, a defer os.Remove(configFile.Name()) should be placed here.
		// However, due to a rclone mount --daemon flag, rclone forks and creates a race condition
		// with this nodeplugin proceess. As a result, the config file gets deleted
		// before it's reread by a forked process.

		if _, err := configFile.Write([]byte(rcloneConfigData)); err != nil {
			return err
		}
		if err := configFile.Close(); err != nil {
			return err
		}

		mountArgs = append(mountArgs, "--config", configFile.Name())
	}
	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(targetPath, 0750)
	if err != nil {
		return err
	}
	klog.Infof("executing mount command cmd=%s, args=%s, targetpath=%s", mountCmd, mountArgs, targetPath)

	cmd := os_exec.Command(mountCmd, mountArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s' remote: '%s' targetpath: %s output: %q",
			err, mountCmd, remoteWithPath, targetPath, string(out))
	}
	return nil
}

func ListSecretsByLabel(ctx context.Context, client *kubernetes.Clientset, namespace string, lab map[string]string) (*corev1.SecretList, error) {
	return client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(lab),
	})
}

func DeleteSecretsByLabel(ctx context.Context, client *kubernetes.Clientset, namespace string, lab map[string]string) error {
	//propagation := metav1.DeletePropagationBackground
	return client.CoreV1().Secrets(namespace).DeleteCollection(ctx, metav1.DeleteOptions{
		//PropagationPolicy: &propagation,
	},
		metav1.ListOptions{
			LabelSelector: labels.FormatLabels(lab),
		})
}

func DeleteDeploymentByLabel(ctx context.Context, client *kubernetes.Clientset, namespace string, lab map[string]string) error {
	propagation := metav1.DeletePropagationForeground
	return client.AppsV1().Deployments(namespace).DeleteCollection(ctx, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	}, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(lab),
	})
}

// func (r *RcloneVolume) normalizedVolumeId() string {
// 	return strings.ToLower(strings.ReplaceAll(r.ID, ":", "-"))
// }

func (r *RcloneVolume) deploymentName() string {
	volumeID := fmt.Sprintf("rclone-mounter-%s", r.ID)
	if len(volumeID) > 63 {
		volumeID = volumeID[:63]
	}

	return strings.ToLower(volumeID)
}

func (r *Rclone) CreateVol(ctx context.Context, volumeName, remote, remotePath, rcloneConfigPath string, parameters map[string]string) error {
	// Create subdirectory under base-dir
	path := fmt.Sprintf("%s/%s", remotePath, volumeName)
	flags := make(map[string]string)
	for key, value := range parameters {
		flags[key] = value
	}
	flags["config"] = rcloneConfigPath

	return r.command("mkdir", remote, path, flags)
}

func (r Rclone) DeleteVol(ctx context.Context, rcloneVolume *RcloneVolume, rcloneConfigPath string, parameters map[string]string) error {
	flags := make(map[string]string)
	for key, value := range parameters {
		flags[key] = value
	}
	flags["config"] = rcloneConfigPath
	return r.command("purge", rcloneVolume.Remote, rcloneVolume.RemotePath, flags)
}

func (r Rclone) Unmount(ctx context.Context, volumeId string, namespace string) error {
	rcloneVolume := &RcloneVolume{ID: volumeId}
	deploymentName := rcloneVolume.deploymentName()
	// Wait for Deployment to stop
	deployment, err := r.kubeClient.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = r.kubeClient.CoreV1().Secrets(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
			if !k8serrors.IsNotFound(err) {
				return err
			}
			return nil
		}
		return err
	}
	opts := metav1.ListOptions{
		TypeMeta:      metav1.TypeMeta{},
		LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
	}
	watcher, err := r.kubeClient.CoreV1().Pods(namespace).Watch(ctx, opts)
	if err != nil {
		return err
	}
	defer watcher.Stop()
	// Delete Deployment
	err = r.kubeClient.AppsV1().Deployments(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	// Block until deployment deleted
	end := false
	klog.Infof("Waiting for pods of deployment/%s to be deleted.", deploymentName)
	for !end {
		select {
		case event := <-watcher.ResultChan():
			if event.Type == watch.Deleted {
				end = true
				klog.Infof("Pods of deployment/%s deleted.", deploymentName)
			}
		case <-ctx.Done():
			end = true
			klog.Infof("Pods deployment/%s waiting context done.", deploymentName)
		}
	}

	return r.kubeClient.CoreV1().Secrets(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})

	/*	labelQuery := map[string]string{
			"volumeid": rcloneVolume.ID,
		}
		err := DeleteDeploymentByLabel(r.kubeClient, namespace, labelQuery)
		if err != nil {
			return err
		}
		return DeleteSecretsByLabel(r.kubeClient, namespace, labelQuery)*/
}

func (r Rclone) CleanupMountPoint(ctx context.Context, secrets, pameters map[string]string) error {
	//TODO implement me
	panic("implement me")
}

func (r Rclone) GetVolumeById(ctx context.Context, volumeId string) (*RcloneVolume, error) {
	pvs, err := r.kubeClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pv := range pvs.Items {
		if pv.Spec.CSI == nil {
			continue
		}
		if pv.Spec.CSI.VolumeHandle == volumeId {
			var remote string
			var path string
			secretRef := pv.Spec.CSI.NodePublishSecretRef
			secrets := make(map[string]string)
			if secretRef != nil {
				sec, err := r.kubeClient.CoreV1().Secrets(secretRef.Namespace).Get(ctx, secretRef.Name, metav1.GetOptions{})
				if err == nil && sec != nil && len(sec.Data) > 0 {
					secrets := make(map[string]string)
					for k, v := range sec.Data {
						secrets[k] = string(v)
					}
				}
			}

			pvcSecret, err := GetPvcSecret(ctx, pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
			if err != nil {
				return nil, err
			}
			remote, path, _, _, err = extractFlags(pv.Spec.CSI.VolumeAttributes, secrets, pvcSecret)
			if err != nil {
				return nil, err
			}

			return &RcloneVolume{
				Remote:     remote,
				RemotePath: path,
				ID:         volumeId,
			}, nil
		}
	}
	return nil, ErrVolumeNotFound
}

func NewRclone(kubeClient *kubernetes.Clientset) Operations {
	return &Rclone{
		execute:    exec.New(),
		kubeClient: kubeClient,
	}
}

func (r *Rclone) command(cmd, remote, remotePath string, flags map[string]string) error {
	// rclone <operand> remote:path [flag]
	args := append(
		[]string{},
		cmd,
		fmt.Sprintf("%s:%s", remote, remotePath),
	)

	// Add user supplied flags
	for k, v := range flags {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}

	klog.Infof("executing %s command cmd=rclone, remote=%s:%s", cmd, remote, remotePath)
	out, err := r.execute.Command("rclone", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %v cmd: 'rclone' remote: '%s' remotePath:'%s' args:'%s'  output: %q",
			cmd, err, remote, remotePath, args, string(out))
	}

	return nil
}

func WaitForPodBySelectorRunning(ctx context.Context, c kubernetes.Interface, namespace, selector string, timeout int) error {
	podList, err := ListPods(ctx, c, namespace, selector)
	if err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods in %s with selector %s", namespace, selector)
	}

	for _, pod := range podList.Items {
		if err := waitForPodRunning(ctx, c, namespace, pod.Name, time.Duration(timeout)*time.Second); err != nil {
			return err
		}
	}
	return nil
}
func ListPods(ctx context.Context, c kubernetes.Interface, namespace, selector string) (*corev1.PodList, error) {
	listOptions := metav1.ListOptions{LabelSelector: selector}
	podList, err := c.CoreV1().Pods(namespace).List(ctx, listOptions)

	if err != nil {
		return nil, err
	}
	return podList, nil
}

func waitForPodRunning(ctx context.Context, c kubernetes.Interface, namespace, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, isPodRunning(ctx, c, podName, namespace))
}

func isPodRunning(ctx context.Context, c kubernetes.Interface, podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		fmt.Printf(".") // progress bar!

		pod, err := c.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		switch pod.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			return false, conditions.ErrPodCompleted
		}
		return false, nil
	}
}
