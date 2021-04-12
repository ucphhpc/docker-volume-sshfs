package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const (
	// VolumeDirMode sets the permissions for the volume directory
	VolumeDirMode = 0700
	// VolumeFileMode sets permissions for the volume files
	VolumeFileMode = 0600
)

type sshfsVolume struct {
	Name       string
	MountPoint string
	CreatedAt  string
	RefCount   int
	// sshfs options
	Options      []string
	SSHCmd       string
	IdentityFile string
	OneTime      bool
	Password     string
	Port         string
}

type sshfsDriver struct {
	mutex      *sync.Mutex
	volumes    map[string]*sshfsVolume
	volumePath string
	statePath  string
}

func (v *sshfsVolume) setupOptions(options map[string]string) error {
	for key, val := range options {
		switch key {
		case "sshcmd":
			v.SSHCmd = val
		case "password":
			v.Password = val
		case "port":
			v.Port = val
		case "IdentityFile":
			v.IdentityFile = val
		case "id_rsa":
			if val != "" {
				v.IdentityFile = v.MountPoint + "_id_rsa"
				if err := v.saveKey(val); err != nil {
					return err
				}
			}
		case "one_time":
			parsedBool, err := strconv.ParseBool(val)
			if err != nil {
				return err
			}
			v.OneTime = parsedBool
		default:
			if val != "" {
				v.Options = append(v.Options, key+"="+val)
			} else {
				v.Options = append(v.Options, key)
			}
		}
	}

	if v.SSHCmd == "" {
		return fmt.Errorf("'sshcmd' option required")
	}

	if v.Password == "" && v.IdentityFile == "" {
		return fmt.Errorf("either 'password', 'IdentityFile' or 'id_rsa' option must be set")
	}

	if v.Password != "" && v.IdentityFile != "" {
		return fmt.Errorf("'password' and 'IdentityFile'/'id_rsa' options are mutually exclusive")
	}

	return nil
}

func (v *sshfsVolume) saveKey(key string) error {
	if key == "" {
		return fmt.Errorf("an empty key is not alloved")
	}

	f, err := os.Create(v.IdentityFile)
	if err != nil {
		msg := fmt.Sprintf("Failed to create id_rsa file at %s (%s)", v.IdentityFile, err)
		log.Error(msg)
		return fmt.Errorf(msg)
	}
	f.WriteString(key)
	f.Chmod(VolumeFileMode)
	f.Close()

	return nil
}

func newSshfsDriver(basePath string) (*sshfsDriver, error) {
	log.Infof("Creating a new driver instance %s", basePath)

	volumePath := filepath.Join(basePath, "volumes")
	statePath := filepath.Join(basePath, "state", "sshfs-state.json")

	if verr := os.MkdirAll(volumePath, VolumeDirMode); verr != nil {
		return nil, verr
	}

	log.Infof("Initialized driver, volumes='%s' state='%s", volumePath, statePath)

	driver := &sshfsDriver{
		volumes:    make(map[string]*sshfsVolume),
		volumePath: volumePath,
		statePath:  statePath,
		mutex:      &sync.Mutex{},
	}

	data, err := ioutil.ReadFile(driver.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debugf("No state found at %s", driver.statePath)
		} else {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(data, &driver.volumes); err != nil {
			return nil, err
		}
	}
	return driver, nil
}

func (d *sshfsDriver) saveState() {
	data, err := json.Marshal(d.volumes)
	if err != nil {
		log.Errorf("saveState failed %s", err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, VolumeFileMode); err != nil {
		log.Errorf("Failed to write state %s to %s (%s)", data, d.statePath, err)
	}
}

// Driver API
func (d *sshfsDriver) Create(r *volume.CreateRequest) error {
	log.Debugf("Create Request %s", r)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol, err := d.newVolume(r.Name)
	if err != nil {
		return err
	}

	if err := vol.setupOptions(r.Options); err != nil {
		return err
	}

	d.volumes[r.Name] = vol
	d.saveState()
	return nil
}

func (d *sshfsDriver) List() (*volume.ListResponse, error) {
	log.Debugf("List Request")

	var vols = []*volume.Volume{}
	for _, vol := range d.volumes {
		vols = append(vols,
			&volume.Volume{Name: vol.Name, Mountpoint: vol.MountPoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *sshfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	log.Debugf("Get Request %s", r)

	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to get volume %s because it doesn't exists", r.Name)
		log.Error(msg)
		return &volume.GetResponse{}, fmt.Errorf(msg)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: vol.Name, Mountpoint: vol.MountPoint}}, nil
}

func (d *sshfsDriver) Remove(r *volume.RemoveRequest) error {
	log.Debugf("Remove Request %s", r)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to remove volume %s because it doesn't exists", r.Name)
		log.Error(msg)
		return fmt.Errorf(msg)
	}

	if vol.RefCount > 0 {
		msg := fmt.Sprintf("Can't remove volume %s because it is mounted by %d containers", vol.Name, vol.RefCount)
		log.Error(msg)
		return fmt.Errorf(msg)
	}

	if err := d.removeVolume(vol); err != nil {
		return err
	}

	delete(d.volumes, vol.Name)
	d.saveState()
	return nil
}

func (d *sshfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	log.Debugf("Path Request %s", r)
	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to find path for volume %s because it doesn't exists", r.Name)
		log.Error(msg)
		return &volume.PathResponse{}, fmt.Errorf(msg)
	}

	return &volume.PathResponse{Mountpoint: vol.MountPoint}, nil
}

func (d *sshfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	log.Debugf("Mount Request %s", r)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to mount volume %s because it doesn't exists", r.Name)
		log.Error(msg)
		return &volume.MountResponse{}, fmt.Errorf(msg)
	}

	if vol.RefCount == 0 {
		log.Debugf("First volume mount %s establish connection to %s", vol.Name, vol.SSHCmd)
		if err := d.mountVolume(vol); err != nil {
			msg := fmt.Sprintf("Failed to mount %s, %s", vol.Name, err)
			log.Error(msg)
			return &volume.MountResponse{}, fmt.Errorf(msg)
		}
	}
	vol.RefCount++
	d.saveState()
	return &volume.MountResponse{Mountpoint: vol.MountPoint}, nil
}

func (d *sshfsDriver) Unmount(r *volume.UnmountRequest) error {
	log.Debugf("Umount Request %s", r)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to unmount volume %s because it doesn't exists", r.Name)
		log.Error(msg)
		return fmt.Errorf(msg)
	}

	vol.RefCount--
	if vol.RefCount <= 0 {
		if err := d.unmountVolume(vol); err != nil {
			return err
		}
		vol.RefCount = 0
	}
	d.saveState()
	return nil
}

func (d *sshfsDriver) Capabilities() *volume.CapabilitiesResponse {
	log.Debugf("Capabilities Request")
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

// Helper methods

func (d *sshfsDriver) newVolume(name string) (*sshfsVolume, error) {
	path := filepath.Join(d.volumePath, name)
	err := os.MkdirAll(path, VolumeDirMode)
	if err != nil {
		msg := fmt.Sprintf("Failed to create the volume mount path %s (%s)", path, err)
		log.Error(msg)
		return nil, fmt.Errorf(msg)
	}

	vol := &sshfsVolume{
		Name:       name,
		MountPoint: path,
		CreatedAt:  time.Now().Format(time.RFC3339Nano),
		OneTime:    false,
		RefCount:   0,
	}
	return vol, nil
}

func (d *sshfsDriver) removeVolume(vol *sshfsVolume) error {
	// Remove id_rsa
	if vol.IdentityFile != "" && vol.OneTime {
		if err := os.Remove(vol.IdentityFile); err != nil {
			msg := fmt.Sprintf("Failed to remove the volume %s id_rsa %s (%s)", vol.Name, vol.MountPoint, err)
			log.Error(msg)
			return fmt.Errorf(msg)
		}
	}

	// Remove MountPoint
	if err := os.Remove(vol.MountPoint); err != nil {
		msg := fmt.Sprintf("Failed to remove the volume %s mountpoint %s (%s)", vol.Name, vol.MountPoint, err)
		log.Error(msg)
		return fmt.Errorf(msg)
	}

	return nil
}

func (d *sshfsDriver) mountVolume(vol *sshfsVolume) error {
	cmd := exec.Command("sshfs", "-oStrictHostKeyChecking=no", vol.SSHCmd, vol.MountPoint)

	if vol.Port != "" {
		cmd.Args = append(cmd.Args, "-p", vol.Port)
	}

	if vol.Password != "" {
		cmd.Args = append(cmd.Args, "-o", "workaround=rename", "-o", "password_stdin")
		cmd.Stdin = strings.NewReader(vol.Password)
	}

	if vol.IdentityFile != "" {
		cmd.Args = append(cmd.Args, "-o", "IdentityFile="+vol.IdentityFile)
	}

	// Append the rest
	for _, option := range vol.Options {
		cmd.Args = append(cmd.Args, "-o", option)
	}

	// Ensure that children have the same process pgid
	log.Debugf("Executing mount command %v", cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sshfs command failed %v (%s)", err, output)
	}

	return nil
}

func (d *sshfsDriver) unmountVolume(vol *sshfsVolume) error {
	cmd := fmt.Sprintf("umount %s", vol.MountPoint)
	if err := exec.Command("sh", "-c", cmd).Run(); err != nil {
		return err
	}
	// Check that the mountpoint is empty
	files, err := ioutil.ReadDir(vol.MountPoint)
	if err != nil {
		return err
	}

	if len(files) > 0 {
		return fmt.Errorf("after unmount %d files still exists in %s", len(files), vol.MountPoint)
	}

	return nil
}
