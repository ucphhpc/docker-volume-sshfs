#!/bin/bash

set -e
set -x

# Defaults
TAG=test

SSH_MOUNT_CONTAINER=$1
SSH_TEST_VOLUME=$2
SSH_MOUNT_PLUGIN=$3

if [ -z "${SSH_MOUNT_CONTAINER}" ]; then
    SSH_MOUNT_CONTAINER=ssh-mount-dummy
fi

if [ -z "${SSH_TEST_VOLUME}" ]; then
    SSH_TEST_VOLUME=ssh-test-volume
fi

if [ -z "${SSH_MOUNT_PLUGIN}" ]; then
    SSH_MOUNT_PLUGIN=ucphhpc/sshfs:${TAG}
fi

DOCKER_SSH_MOUNT_IMAGE=ucphhpc/ssh-mount-dummy
TEST_SSH_KEY_DIRECTORY=`pwd`/.gocd/ssh
TEST_SSH_KEY_PATH=${TEST_SSH_KEY_DIRECTORY}/id_rsa

MOUNT_USER=mountuser
MOUNT_PASSWORD=Passw0rd!
MOUNT_HOST=localhost
MOUNT_PATH=/home/${MOUNT_USER}
MOUNT_PORT=2222

# install
docker pull ${DOCKER_SSH_MOUNT_IMAGE}
docker pull busybox

# Remove any conflicting docker items
make testclean clean TAG=${TAG}

# make the plugin
make TAG=${TAG}
# enable the plugin
make enable TAG=${TAG}

# start sshd
docker run -d -p ${MOUNT_PORT}:22 --name ${SSH_MOUNT_CONTAINER} ${DOCKER_SSH_MOUNT_IMAGE}
# It takes a while for the container to start and be ready to accept connection
# TODO, check when container is ready instead of sleeping
sleep 20

echo "------------ test 1 simple password ------------\n"

# test1: simple
docker volume create -d ${SSH_MOUNT_PLUGIN} -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o port=${MOUNT_PORT} -o password=${MOUNT_PASSWORD} ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

echo "------------ test 2 allow_other ------------\n"

# test2: allow_other
docker volume create -d ${SSH_MOUNT_PLUGIN} -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o allow_other -o port=${MOUNT_PORT} -o password=${MOUNT_PASSWORD} ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write -u nobody busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read -u nobody busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

echo "------------ test 3 compression ------------\n"

# test3: compression
docker volume create -d ${SSH_MOUNT_PLUGIN} -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o Ciphers=chacha20-poly1305@openssh.com -o Compression=no -o port=$MOUNT_PORT -o password=$MOUNT_PASSWORD ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

echo "------------ test 4 source ------------\n"

# test4: source
docker plugin disable ${SSH_MOUNT_PLUGIN}
docker plugin set ${SSH_MOUNT_PLUGIN} state.source=/tmp
docker plugin enable ${SSH_MOUNT_PLUGIN}
docker volume create -d ${SSH_MOUNT_PLUGIN} -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o Ciphers=chacha20-poly1305@openssh.com -o Compression=no -o port=$MOUNT_PORT -o password=$MOUNT_PASSWORD ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

# Cleanup
make testclean clean TAG=${TAG} TEST_SSH_MOUNT_CONTAINER=${SSH_MOUNT_CONTAINER} TEST_SSH_VOLUME=${SSH_TEST_VOLUME}
