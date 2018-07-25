package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
	"os"
)

const (
	DefaultBaseVolumePath = "/mnt/volumes"
	DefaultBaseMountPath = "/mnt/mounts"
	DefaultUnixSocket = "/run/docker/plugins/" + DriverName + ".sock"
)

func main() {
	driver, err := newSshfsDriver(DefaultBaseVolumePath, DefaultBaseMountPath)
	if err != nil {
		log.Errorf("Failed to create the driver %s", err)
		os.Exit(1)
	}
	log.SetLevel(log.DebugLevel)
	handler := volume.NewHandler(driver)
	handler.ServeUnix(DefaultUnixSocket, 0)
}