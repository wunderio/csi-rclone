package rclone

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	os_exec "os/exec"
	"syscall"
	"time"

	"strings"

	"golang.org/x/net/context"
	"gopkg.in/ini.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/utils/exec"
)

var (
	ErrVolumeNotFound = errors.New("volume is not found")
)

type Operations interface {
	CreateVol(ctx context.Context, volumeName, remote, remotePath, rcloneConfigPath string, pameters map[string]string) error
	DeleteVol(ctx context.Context, rcloneVolume *RcloneVolume, rcloneConfigPath string, pameters map[string]string) error
	Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath string, namespace string, rcloneConfigData string, readOnly bool, pameters map[string]string) error
	Unmount(ctx context.Context, volumeId string, targetPath string) error
	GetVolumeById(ctx context.Context, volumeId string) (*RcloneVolume, error)
	Cleanup()
}

type Rclone struct {
	execute    exec.Interface
	kubeClient *kubernetes.Clientset
	process    int
}

type RcloneVolume struct {
	Remote     string
	RemotePath string
	ID         string
}
type MountRequest struct {
	Fs         string   `json:"fs"`
	MountPoint string   `json:"mountPoint"`
	VfsOpt     VfsOpt   `json:"vfsOpt"`
	MountOpt   MountOpt `json:"mountOpt"`
}

type VfsOpt struct {
	CacheMode    string        `json:"cacheMode"`
	DirCacheTime time.Duration `json:"dirCacheTime"`
	ReadOnly     bool          `json:"readOnly"`
}
type MountOpt struct {
	AllowNonEmpty bool `json:"allowNonEmpty"`
	AllowOther    bool `json:"allowOther"`
}
type ConfigCreateRequest struct {
	Name        string                 `json:"name"`
	Parameters  map[string]string      `json:"parameters"`
	StorageType string                 `json:"type"`
	Opt         map[string]interface{} `json:"opt"`
}

type UnmountRequest struct {
	MountPoint string `json:"mountPoint"`
}
type ConfigDeleteRequest struct {
	Name string `json:"name"`
}

func (r *Rclone) Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath, namespace string, rcloneConfigData string, readOnly bool, parameters map[string]string) error {
	configName := rcloneVolume.deploymentName()
	cfg, err := ini.Load([]byte(rcloneConfigData))
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't load config %s", err)
	}
	secs := cfg.Sections()
	if len(secs) != 2 { //there's also a DEFAULT section
		return fmt.Errorf("Mounting failed: expected only one config section: %s", cfg.SectionStrings())
	}
	sec := secs[1]
	params := make(map[string]string)
	for _, key := range sec.KeyStrings() {
		if key == "type" {
			continue
		}
		params[key] = sec.Key(key).String()
	}
	configOpts := ConfigCreateRequest{
		Name:        configName,
		StorageType: sec.Key("type").String(),
		Parameters:  params,
		Opt:         map[string]interface{}{"obscure": true},
	}
	klog.Infof("executing create config command  args=%v, targetpath=%s", configName, targetPath)
	postBody, err := json.Marshal(configOpts)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create request body: %s", err)
	}
	requestBody := bytes.NewBuffer(postBody)
	resp, err := http.Post("http://localhost:5572/config/create", "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("mounting failed:  err: %s", err)
	}
	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("mounting failed: couldn't create config: %s", string(body))
	}
	klog.Infof("created config: %s", configName)

	remoteWithPath := fmt.Sprintf("%s:%s", configName, rcloneVolume.RemotePath)
	mountArgs := MountRequest{
		Fs:         remoteWithPath,
		MountPoint: targetPath,
		VfsOpt: VfsOpt{
			CacheMode:    "writes",
			DirCacheTime: 60 * time.Second,
			ReadOnly:     readOnly,
		},
		MountOpt: MountOpt{
			AllowNonEmpty: true,
			AllowOther:    true,
		},
	}

	// create target, os.Mkdirall is noop if it exists
	err = os.MkdirAll(targetPath, 0750)
	if err != nil {
		return err
	}
	klog.Infof("executing mount command  args=%v, targetpath=%s", mountArgs, targetPath)
	postBody, err = json.Marshal(mountArgs)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create request body: %s", err)
	}
	requestBody = bytes.NewBuffer(postBody)
	resp, err = http.Post("http://localhost:5572/mount/mount", "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("mounting failed:  err: %s", err)
	}
	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("mounting failed: couldn't create mount: %s", string(body))
	}
	klog.Infof("created mount: %s", configName)

	defer resp.Body.Close()

	return nil
}

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

func (r Rclone) Unmount(ctx context.Context, volumeId string, targetPath string) error {
	rcloneVolume := &RcloneVolume{ID: volumeId}

	klog.Infof("unmounting %s", rcloneVolume.deploymentName())
	unmountArgs := UnmountRequest{
		MountPoint: targetPath,
	}
	postBody, err := json.Marshal(unmountArgs)
	if err != nil {
		return fmt.Errorf("unmounting failed: couldn't create request body: %s", err)
	}
	requestBody := bytes.NewBuffer(postBody)
	resp, err := http.Post("http://localhost:5572/mount/unmount", "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("unmounting failed:  err: %s", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		errorMsg, found := result["error"]
		if !found || errorMsg != "mount not found" {
			// if the mount errored out, we'd get mount not found and want to continue
			return fmt.Errorf("unmounting failed: couldn't delete mount: %s", string(body))
		}
	}
	klog.Infof("deleted mount: %s", string(body))
	defer resp.Body.Close()

	configDelete := ConfigDeleteRequest{
		Name: rcloneVolume.deploymentName(),
	}
	postBody, err = json.Marshal(configDelete)
	if err != nil {
		return fmt.Errorf("deleting config failed: couldn't create request body: %s", err)
	}
	requestBody = bytes.NewBuffer(postBody)
	resp, err = http.Post("http://localhost:5572/config/delete", "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("deleting config failed:  err: %s", err)
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		//don't error here so storage can be successfully unmounted
		klog.Errorf("deleting config failed: couldn't delete mount: %s", string(body))
	}
	klog.Infof("deleted config: %s", string(body))

	return nil
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
	rclone := &Rclone{
		execute:    exec.New(),
		kubeClient: kubeClient,
	}
	err := rclone.run_daemon()
	if err != nil {
		panic(fmt.Sprintf("couldn't start backround process: %s", err))
	}

	return rclone
}

func (r *Rclone) run_daemon() error {
	f, err := os.CreateTemp("", "rclone.conf")
	if err != nil {
		return err
	}
	rclone_cmd := "rclone"
	rclone_args := []string{}
	rclone_args = append(rclone_args, "rcd")
	rclone_args = append(rclone_args, "--rc-addr=:5572")
	rclone_args = append(rclone_args, "--cache-info-age=72h")
	rclone_args = append(rclone_args, "--cache-chunk-clean-interval=15m")
	rclone_args = append(rclone_args, "--rc-no-auth")
	rclone_args = append(rclone_args, "--log-file=/tmp/rclone.log")
	rclone_args = append(rclone_args, fmt.Sprintf("--config=%s", f.Name()))
	klog.Infof("running rclone remote control daemon cmd=%s, args=%s, ", rclone_cmd, rclone_args)

	env := os.Environ()
	cmd := os_exec.Command(rclone_cmd, rclone_args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic("couldn't get stderr of rclone process")
	}
	scanner := bufio.NewScanner(stderr)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	r.process = cmd.Process.Pid
	go func() {
		output := ""
		for scanner.Scan() {
			output = scanner.Text()
		}
		err := cmd.Wait()
		if err != nil {
			klog.Errorf("background process failed with: %s,%s", output, err)
		}
	}()
	return nil
}
func (r *Rclone) Cleanup() {
	klog.Info("cleaning up background process")
	p, err := os.FindProcess(r.process)
	if err != nil {
		return
	}
	p.Signal(syscall.SIGINT)
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
