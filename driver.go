package main

import (
	"os"
	"sync"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
	"path/filepath"
	"fmt"
	"os/exec"
	"time"
	"strings"
)

const (
	DriverName		= "sshfs"
	VolumeDirMode	= 0755
)

type sshfsVolume struct {
	Name		string
	MountPoint	string
	CreatedAt	string
	refCount 	int
	// sshfs options
	Options		[]string
	SSHCmd		string
	Password	string
	Port		string
}

type sshfsDriver struct {
	mutex			*sync.Mutex
	volumes			map[string]*sshfsVolume
	baseVolumePath	string
	baseMountPath	string
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

	return nil
}

func newSshfsDriver(baseVolumePath string, baseMountPath string) (*sshfsDriver, error) {
	log.Info("Creating a new driver instance")

	verr := os.MkdirAll(baseVolumePath, VolumeDirMode)
	if verr != nil {
		return nil, verr
	}

	merr := os.MkdirAll(baseMountPath, VolumeDirMode)
	if merr != nil {
		return nil, merr
	}

	log.Infof("Initialized driver state, volumes='%s' mounts='%s'",
		baseVolumePath, baseMountPath)

	driver := &sshfsDriver{
		volumes:		make(map[string]*sshfsVolume),
		baseVolumePath:	baseVolumePath,
		baseMountPath:	baseMountPath,
		mutex:			&sync.Mutex{},
	}
	//TODO Check for exisiting volumes
	return driver, nil
}

// Driver API

func (d *sshfsDriver) Create(r *volume.CreateRequest) error {
	log.Debugf("Create Request %s", r)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vol, err := d.initVolume(r.Name)
	if err != nil {
		return err
	}

	if err := vol.setupOptions(r.Options); err != nil {
		return err
	}

	// TODO, do a mount test

	d.volumes[r.Name] = vol
	return nil
}

func (d *sshfsDriver) List() (*volume.ListResponse, error) {
	log.Debugf("List Request")

	var vols = []*volume.Volume{}
	for _, vol := range d.volumes {
		vols = append(vols,
			&volume.Volume{Name: vol.Name, Mountpoint: vol.MountPoint, CreatedAt: vol.CreatedAt})
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

	return &volume.GetResponse{Volume:
		&volume.Volume{Name: vol.Name, Mountpoint: vol.MountPoint, CreatedAt: vol.CreatedAt}}, nil
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

	if vol.refCount != 0 {
		msg := fmt.Sprintf("Can't remove volume %s because it is mounted by %i containers", vol.Name, vol.refCount)
		log.Error(msg)
		return fmt.Errorf(msg)
	}

	if err := d.removeVolume(vol); err != nil {
		return err
	}

	delete(d.volumes, vol.Name)
	return nil
}

func (d *sshfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	log.Debugf("Path Request %s", r)

	vol, ok := d.volumes[r.Name]
	if !ok {
		msg := fmt.Sprintf("Failed to find path for volume %s because it doesn't exists %s", r.Name)
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
		msg := fmt.Sprintf("Failed to mount volume %s because it doesn't exists %s", r.Name)
		log.Error(msg)
		return &volume.MountResponse{}, fmt.Errorf(msg)
	}

	if vol.refCount == 0 {
		log.Debugf("First volume reference %s", vol.Name)
		if merr := d.mountVolume(vol); merr != nil {
			msg := fmt.Sprintf("Failed to mount %s, %s", vol.Name, merr)
			log.Error(msg)
			return &volume.MountResponse{}, fmt.Errorf(msg)
		}
	}
	vol.refCount++
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

	vol.refCount--

	if vol.refCount == 0 {
		if err := d.unmountVolume(vol.MountPoint); err != nil {
			return err
		}
	}

	return nil
}

func (d *sshfsDriver) Capabilities() *volume.CapabilitiesResponse {
	log.Debugf("Capabilities Request")
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

// Helper methods

func (d *sshfsDriver) initVolume(name string) (*sshfsVolume, error) {
	path := filepath.Join(d.baseVolumePath, name)

	merr := os.MkdirAll(path, VolumeDirMode)
	if merr != nil {
		msg := fmt.Sprintf("Failed to create the volume mount path %s", path)
		log.Error(msg)
		return nil, fmt.Errorf(msg)
	}

	d.unmountVolume(path)

	vol := &sshfsVolume{
		Name: name,
		MountPoint: path,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}

	return vol, nil
}

func (d *sshfsDriver) removeVolume(vol *sshfsVolume) error {
	// Remove MountPoint
	if rerr := os.RemoveAll(vol.MountPoint); rerr != nil {
		msg := fmt.Sprintf("Failed to remove the volume %s mountpoint %s", vol.Name, vol.MountPoint)
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

	// Append the rest
	for _, option := range vol.Options {
		cmd.Args = append(cmd.Args, "-o", option)
	}
	log.Debugf("Executing mount command %v", cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sshfs command failed %v (%s)", err, output)
	}

	return nil
}

func (d *sshfsDriver) unmountVolume(path string) error {
	cmd := fmt.Sprintf("umount %s", path)
	return exec.Command("sh", "-c", cmd).Run()
}