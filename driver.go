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
	VolumeDirMode	= 0700
	VolumeFileMode  = 0600
)

type sshfsVolume struct {
	Name		string
	MountPoint	string
	CreatedAt	string
	RefCount 	int
	// sshfs options
	Options		[]string
	SSHCmd		string
	IDRsa    	string
	Password	string
	Port		string
}

type sshfsDriver struct {
	mutex			*sync.Mutex
	volumes			map[string]*sshfsVolume
	baseVolumePath	string
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
		case "id_rsa":
			v.IDRsa = val
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

	if v.Password == "" && v.IDRsa == "" {
		return fmt.Errorf("either 'password' or 'id_rsa' option must be set")
	}

	if v.Password != "" && v.IDRsa != "" {
		return fmt.Errorf("'password' and 'id_rsa' options are mutually exclusive")
	}

	return nil
}

func (v *sshfsVolume) initVolume() error {
	if v.IDRsa != "" {
		idRsa := v.MountPoint + "_id_rsa"
		if f, err := os.Create(idRsa); err != nil {
			msg := fmt.Sprintf("Failed to create id_rsa file at %s (%s)", idRsa, err)
			log.Error(msg)
			return fmt.Errorf(msg)
		} else {
			f.WriteString(v.IDRsa)
			f.Chmod(VolumeFileMode)
			f.Close()
		}
	}
	return  nil
}


func newSshfsDriver(baseVolumePath string) (*sshfsDriver, error) {
	log.Info("Creating a new driver instance")

	verr := os.MkdirAll(baseVolumePath, VolumeDirMode)
	if verr != nil {
		return nil, verr
	}

	log.Infof("Initialized driver state, volumes='%s'", baseVolumePath)

	driver := &sshfsDriver{
		volumes:		make(map[string]*sshfsVolume),
		baseVolumePath:	baseVolumePath,
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

	vol, err := d.newVolume(r.Name)
	if err != nil {
		return err
	}

	if err := vol.setupOptions(r.Options); err != nil {
		return err
	}

	if err := vol.initVolume(); err != nil {
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

	if vol.RefCount != 0 {
		msg := fmt.Sprintf("Can't remove volume %s because it is mounted by %i containers", vol.Name, vol.RefCount)
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

	if vol.RefCount == 0 {
		log.Debugf("First volume reference %s", vol.Name)
		if merr := d.mountVolume(vol); merr != nil {
			msg := fmt.Sprintf("Failed to mount %s, %s", vol.Name, merr)
			log.Error(msg)
			return &volume.MountResponse{}, fmt.Errorf(msg)
		}
	}
	vol.RefCount++
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

	return nil
}

func (d *sshfsDriver) Capabilities() *volume.CapabilitiesResponse {
	log.Debugf("Capabilities Request")
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "global"}}
}

// Helper methods

func (d *sshfsDriver) newVolume(name string) (*sshfsVolume, error) {
	path := filepath.Join(d.baseVolumePath, name)

	err := os.MkdirAll(path, VolumeDirMode)
	if err != nil {
		msg := fmt.Sprintf("Failed to create the volume mount path %s (%s)", path, err)
		log.Error(msg)
		return nil, fmt.Errorf(msg)
	}

	vol := &sshfsVolume{
		Name: name,
		MountPoint: path,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}
	// Ensure mount is not active
	d.unmountVolume(vol)

	return vol, nil
}

func (d *sshfsDriver) removeVolume(vol *sshfsVolume) error {
	// Remove id_rsa
	if vol.IDRsa != "" {
		if err := os.RemoveAll(vol.MountPoint + "_id_rsa"); err != nil {
			msg := fmt.Sprintf("Failed to remove the volume %s id_rsa %s (%s)", vol.Name, vol.MountPoint, err)
			log.Error(msg)
			return fmt.Errorf(msg)
		}
	}

	// Remove MountPoint
	if  err := os.RemoveAll(vol.MountPoint); err != nil {
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

	if vol.IDRsa != "" {
		cmd.Args = append(cmd.Args, "-o", "IdentityFile=" + vol.MountPoint + "_id_rsa")
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
	return exec.Command("sh", "-c", cmd).Run()
}