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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath string, rcloneConfigData string, readOnly bool, pameters map[string]string) error
	Unmount(ctx context.Context, volumeId string, targetPath string) error
	GetVolumeById(ctx context.Context, volumeId string) (*RcloneVolume, error)
	Cleanup() error
	Run() error
}

type Rclone struct {
	execute    exec.Interface
	kubeClient *kubernetes.Clientset
	daemonCmd  *os_exec.Cmd
	port       int
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

// VfsOpt is options for creating the vfs
//
// Note that the `Daemon` option has been removed as it is not accepted for rc calls.
type VfsOpt struct {
	NoSeek             bool          `json:",omitempty"` // don't allow seeking if set
	NoChecksum         bool          `json:",omitempty"` // don't check checksums if set
	ReadOnly           bool          `json:",omitempty"` // if set VFS is read only
	NoModTime          bool          `json:",omitempty"` // don't read mod times for files
	DirCacheTime       time.Duration `json:",omitempty"` // how long to consider directory listing cache valid
	Refresh            bool          `json:",omitempty"` // refreshes the directory listing recursively on start
	PollInterval       time.Duration `json:",omitempty"`
	Umask              int           `json:",omitempty"`
	UID                uint32        `json:",omitempty"`
	GID                uint32        `json:",omitempty"`
	DirPerms           os.FileMode   `json:",omitempty"`
	FilePerms          os.FileMode   `json:",omitempty"`
	ChunkSize          int64         `json:",omitempty"` // if > 0 read files in chunks
	ChunkSizeLimit     int64         `json:",omitempty"` // if > ChunkSize double the chunk size after each chunk until reached
	CacheMode          string        `json:",omitempty"`
	CacheMaxAge        time.Duration `json:",omitempty"`
	CacheMaxSize       int64         `json:",omitempty"`
	CacheMinFreeSpace  int64         `json:",omitempty"`
	CachePollInterval  time.Duration `json:",omitempty"`
	CaseInsensitive    bool          `json:",omitempty"`
	WriteWait          time.Duration `json:",omitempty"` // time to wait for in-sequence write
	ReadWait           time.Duration `json:",omitempty"` // time to wait for in-sequence read
	WriteBack          time.Duration `json:",omitempty"` // time to wait before writing back dirty files
	ReadAhead          int64         `json:",omitempty"` // bytes to read ahead in cache mode "full"
	UsedIsSize         bool          `json:",omitempty"` // if true, use the `rclone size` algorithm for Used size
	FastFingerprint    bool          `json:",omitempty"` // if set use fast fingerprints
	DiskSpaceTotalSize int64         `json:",omitempty"`
}

// Options for creating the mount
//
// Note that options not supported on Linux have been removed.
type MountOpt struct {
	DebugFUSE          bool          `json:",omitempty"`
	AllowNonEmpty      bool          `json:",omitempty"`
	AllowRoot          bool          `json:",omitempty"`
	AllowOther         bool          `json:",omitempty"`
	DefaultPermissions bool          `json:",omitempty"`
	WritebackCache     bool          `json:",omitempty"`
	DaemonWait         time.Duration `json:",omitempty"` // time to wait for ready mount from daemon, maximum on Linux or constant on macOS/BSD
	MaxReadAhead       int64         `json:",omitempty"`
	ExtraOptions       []string      `json:",omitempty"`
	ExtraFlags         []string      `json:",omitempty"`
	AttrTimeout        time.Duration `json:",omitempty"` // how long the kernel caches attribute for
	DeviceName         string        `json:",omitempty"`
	VolumeName         string        `json:",omitempty"`
	NoAppleDouble      bool          `json:",omitempty"`
	NoAppleXattr       bool          `json:",omitempty"`
	AsyncRead          bool          `json:",omitempty"`
	CaseInsensitive    string        `json:",omitempty"`
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

func (r *Rclone) Mount(ctx context.Context, rcloneVolume *RcloneVolume, targetPath, rcloneConfigData string, readOnly bool, parameters map[string]string) error {
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
	params["config_refresh_token"] = "false"
	configOpts := ConfigCreateRequest{
		Name:        configName,
		StorageType: sec.Key("type").String(),
		Parameters:  params,
		Opt:         map[string]interface{}{"obscure": true},
	}
	klog.Infof("executing create config command name=%s, storageType=%s", configName, configOpts.StorageType)
	postBody, err := json.Marshal(configOpts)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create request body: %s", err)
	}
	requestBody := bytes.NewBuffer(postBody)
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/config/create", r.port), "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't send HTTP request to create config: %w", err)
	}
	err = checkResponse(resp)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create config: %w", err)
	}
	klog.Infof("created config: %s", configName)

	// VFS Mount parameters
	vfsOpt := VfsOpt{
		CacheMode:    "writes",
		DirCacheTime: 60 * time.Second,
	}
	vfsOptStr := parameters["vfsOpt"]
	if vfsOptStr != "" {
		err = json.Unmarshal([]byte(vfsOptStr), &vfsOpt)
		if err != nil {
			return fmt.Errorf("could not parse vfsOpt: %w", err)
		}
	}
	// The `ReadOnly` option is specified in the PVC
	vfsOpt.ReadOnly = readOnly
	// Mount parameters
	mountOpt := MountOpt{
		AllowNonEmpty: true,
		AllowOther:    true,
	}
	mountOptStr := parameters["mountOpt"]
	if mountOptStr != "" {
		err = json.Unmarshal([]byte(mountOptStr), &mountOpt)
		if err != nil {
			return fmt.Errorf("could not parse mountOpt: %w", err)
		}
	}

	remoteWithPath := fmt.Sprintf("%s:%s", configName, rcloneVolume.RemotePath)
	mountArgs := MountRequest{
		Fs:         remoteWithPath,
		MountPoint: targetPath,
		VfsOpt:     vfsOpt,
		MountOpt:   mountOpt,
	}

	// create target, os.Mkdirall is noop if it exists
	err = os.MkdirAll(targetPath, 0750)
	if err != nil {
		return err
	}
	postBody, err = json.Marshal(mountArgs)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create request body: %s", err)
	}
	klog.Infof("executing mount command args=%s", string(postBody))
	requestBody = bytes.NewBuffer(postBody)
	resp, err = http.Post(fmt.Sprintf("http://localhost:%d/mount/mount", r.port), "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't send HTTP request to create mount: %w", err)
	}
	err = checkResponse(resp)
	if err != nil {
		return fmt.Errorf("mounting failed: couldn't create mount: %w", err)
	}
	klog.Infof("created mount: %s", configName)

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
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/mount/unmount", r.port), "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("unmounting failed: couldn't send HTTP request: %w", err)
	}
	err = checkResponse(resp)
	if err != nil {
		return fmt.Errorf("unmounting failed: %w", err)
	}
	klog.Infof("deleted mount with volume ID %s at path %s", volumeId, targetPath)

	configDelete := ConfigDeleteRequest{
		Name: rcloneVolume.deploymentName(),
	}
	postBody, err = json.Marshal(configDelete)
	if err != nil {
		return fmt.Errorf("deleting config failed: couldn't create request body: %s", err)
	}
	requestBody = bytes.NewBuffer(postBody)
	resp, err = http.Post(fmt.Sprintf("http://localhost:%d/config/delete", r.port), "application/json", requestBody)
	if err != nil {
		klog.Errorf("deleting config failed: couldn't send HTTP request: %v", err)
		return nil
	}
	err = checkResponse(resp)
	if err != nil {
		klog.Errorf("deleting config failed: %v", err)
		return nil
	}
	klog.Infof("deleted config for volume ID %s at path %s", volumeId, targetPath)

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
						// Note you have to decode the secret here
						secrets[k] = string(v)
					}
				}
			}

			// This is for compatibility reasons, in the old version the PVC secret was the same name as the PVC
			// Now the secret is taken from the PVC annotation and injected in the `secrets` map above
			pvcSecret, err := getSecret(ctx, pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}
			remote, path, _, _, err = extractFlags(pv.Spec.CSI.VolumeAttributes, secrets, pvcSecret, nil)
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

func NewRclone(kubeClient *kubernetes.Clientset, port int) Operations {
	rclone := &Rclone{
		execute:    exec.New(),
		kubeClient: kubeClient,
		port:       port,
	}
	return rclone
}

// Format from https://rclone.org/rc/#error-returns
type serverErrorResponse struct {
	Error  string          `json:"error"`
	Path   string          `json:"path"`
	input  json.RawMessage // can contain sensitive info in plain text
	status int             // same as the http status code
}

func (s serverErrorResponse) String() string {
	return fmt.Sprintf(
		"{%q: %q, %q: %q, %q: %q, %q: %d}",
		"error",
		s.Error,
		"path",
		s.Path,
		"input",
		"<redacted>",
		"status",
		s.status,
	)
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// everything is ok with the response, there should be no error
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		// NOTE: do not wrap the error in case it contains sensitive information from the body
		return fmt.Errorf("could not read the error response body from the rclone server")
	}
	var result serverErrorResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		// NOTE: do not wrap the error in case it contains sensitive information from the body
		return fmt.Errorf("could not unmarshal the error response from the rclone server")
	}
	if result.Error == "" {
		return fmt.Errorf("unmarshalled the response from the server but it had nothing in the error field")
	}
	return fmt.Errorf("received error from the rclone server: %s", result.String())
}

func (r *Rclone) start_daemon() error {
	f, err := os.CreateTemp("", "rclone.conf")
	if err != nil {
		return err
	}
	rclone_cmd := "rclone"
	rclone_args := []string{}
	rclone_args = append(rclone_args, "rcd")
	rclone_args = append(rclone_args, fmt.Sprintf("--rc-addr=:%d", r.port))
	rclone_args = append(rclone_args, "--cache-info-age=72h")
	rclone_args = append(rclone_args, "--cache-chunk-clean-interval=15m")
	rclone_args = append(rclone_args, "--rc-no-auth")
	loglevel := os.Getenv("LOG_LEVEL")
	if len(loglevel) == 0 {
		loglevel = "NOTICE"
	}
	rclone_args = append(rclone_args, fmt.Sprintf("--log-level=%s", loglevel))
	rclone_args = append(rclone_args, fmt.Sprintf("--config=%s", f.Name()))
	klog.Infof("running rclone remote control daemon cmd=%s, args=%s, ", rclone_cmd, rclone_args)

	env := os.Environ()
	cmd := os_exec.Command(rclone_cmd, rclone_args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err != nil {
		panic("couldn't get stderr of rclone process")
	}
	scanner := bufio.NewScanner(stdout)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		output := ""
		for scanner.Scan() {
			output = scanner.Text()
			klog.Infof("rclone log: %s", output)
		}
	}()
	r.daemonCmd = cmd
	return nil
}

func (r *Rclone) Run() error {
	err := r.start_daemon()
	if err != nil {
		return err
	}
	// blocks until the rclone daemon is stopped
	return r.daemonCmd.Wait()
}

func (r *Rclone) Cleanup() error {
	klog.Info("cleaning up background process")
	if r.daemonCmd == nil {
		return nil
	}
	return r.daemonCmd.Process.Kill()
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
