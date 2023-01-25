#!/bin/bash

set -e
set -x

# Defaults
TAG=test

DOCKER_SSH_MOUNT_IMAGE=ucphhpc/ssh-mount-dummy
SSH_MOUNT_CONTAINER=ssh-mount-dummy-key
SSH_TEST_VOLUME=ssh-test-volume-key
SSH_MOUNT_PLUGIN=ucphhpc/sshfs:$TAG

TEST_SSH_KEY_DIRECTORY=`pwd`/.gocd/ssh
TEST_SSH_KEY_PATH=$TEST_SSH_KEY_DIRECTORY/id_rsa

MOUNT_USER=mountuser
MOUNT_HOST=localhost
MOUNT_PATH=/home/$MOUNT_USER
MOUNT_PORT=2222

# install
docker pull $DOCKER_SSH_MOUNT_IMAGE
docker pull busybox

# Generate new SSH keypair for testing if it doesn't already exist
# Has no password
if [ ! -d $TEST_SSH_KEY_DIRECTORY ]; then
    mkdir -p $TEST_SSH_KEY_DIRECTORY
fi

if [ ! -r $TEST_SSH_KEY_PATH ]; then
    ssh-keygen -t rsa -N "" -f $TEST_SSH_KEY_PATH
fi

# Read in the public key
MOUNT_SSH_PUB_KEY_CONTENT=`cat $TEST_SSH_KEY_PATH`

# make the plugin
PLUGIN_TAG=$TAG make
# enable the plugin
docker plugin enable $SSH_MOUNT_PLUGIN
# list plugins
docker plugin ls
# start sshd
docker run -d -p $MOUNT_PORT:22 --name $SSH_MOUNT_CONTAINER $DOCKER_SSH_MOUNT_IMAGE
# Copy in the public key
docker exec -it $SSH_MOUNT_CONTAINER bash -c "echo $MOUNT_SSH_PUB_KEY_CONTENT >> $MOUNT_PATH/.ssh/authorized_keys"

# It takes a while for the container to start and be ready to accept connection
# TODO, check when container is ready instead of sleeping
sleep 20

# test1: ssh key
docker plugin disable $SSH_MOUNT_PLUGIN
docker plugin set $SSH_MOUNT_PLUGIN sshkey.source=$TEST_SSH_KEY_DIRECTORY
docker plugin enable $SSH_MOUNT_PLUGIN
docker volume create -d $SSH_MOUNT_PLUGIN -o IdentityFile=/root/.ssh/id_rsa -o sshcmd=$MOUNT_USER@$MOUNT_HOST:$MOUNT_PATH -o port=$MOUNT_PORT $SSH_TEST_VOLUME
docker run --rm -v $SSH_TEST_VOLUME:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v $SSH_TEST_VOLUME:/read busybox grep -Fxq hello /read/world
docker volume rm $SSH_TEST_VOLUME

# test2: ssh id_rsa flag
docker volume create -d $SSH_MOUNT_PLUGIN -o sshcmd=$MOUNT_USER@$MOUNT_HOST:$MOUNT_PATH -o port=$MOUNT_PORT -o id_rsa="$(cat $TEST_SSH_KEY_PATH)" $SSH_TEST_VOLUME
docker run --rm -v $SSH_TEST_VOLUME:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v $SSH_TEST_VOLUME:/read busybox grep -Fxq hello /read/world
docker volume rm $SSH_TEST_VOLUME

# remove the test mount container
docker stop $SSH_MOUNT_CONTAINER
docker rm $SSH_MOUNT_CONTAINER

# remove the plugin
docker plugin disable $SSH_MOUNT_PLUGIN
docker plugin remove $SSH_MOUNT_PLUGIN