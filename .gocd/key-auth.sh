#!/bin/bash

set -e
set -x

# Defaults
TAG=test

SSH_MOUNT_CONTAINER=$1
SSH_TEST_VOLUME=$2
SSH_MOUNT_PLUGIN=$3

if [ -z "${SSH_MOUNT_CONTAINER}" ]; then
    SSH_MOUNT_CONTAINER="ssh-mount-dummy-key"
fi

if [ -z "${SSH_TEST_VOLUME}" ]; then
    SSH_TEST_VOLUME="ssh-test-volume-key"
fi

if [ -z "${SSH_MOUNT_PLUGIN}" ]; then
    SSH_MOUNT_PLUGIN="ucphhpc/sshfs:${TAG}"
fi

DOCKER_SSH_MOUNT_IMAGE="ucphhpc/ssh-mount-dummy"
TEST_SSH_KEY_DIRECTORY=`pwd`/.gocd/ssh
TEST_SSH_KEY_PATH=${TEST_SSH_KEY_DIRECTORY}/id_rsa

MOUNT_USER=mountuser
MOUNT_HOST=localhost
MOUNT_PATH=/home/${MOUNT_USER}
MOUNT_PORT=2222

# install
docker pull ${DOCKER_SSH_MOUNT_IMAGE}
docker pull busybox

# Generate new SSH keypair for testing if it doesn't already exist
# Has no password
if [ ! -d ${TEST_SSH_KEY_DIRECTORY} ]; then
    mkdir -p ${TEST_SSH_KEY_DIRECTORY}
fi

if [ ! -r ${TEST_SSH_KEY_PATH} ]; then
    ssh-keygen -t rsa -N "" -f ${TEST_SSH_KEY_PATH}
fi

# Read in the public key
MOUNT_SSH_PUB_KEY_CONTENT=`cat ${TEST_SSH_KEY_PATH}.pub`

# make the plugin
make TAG="${TAG}"
# enable the plugin
make enable TAG="${TAG}"

# start sshd
docker run -d -p ${MOUNT_PORT}:22 --name ${SSH_MOUNT_CONTAINER} ${DOCKER_SSH_MOUNT_IMAGE}
# It takes a while for the container to start and be ready to accept connection
# TODO, check when container is ready instead of sleeping
sleep 20

# Copy in the public key
# Write a newline followed by the public key
# https://unix.stackexchange.com/questions/191694/how-to-put-a-newline-special-character-into-a-file-using-the-echo-command-and-re
docker exec -it ${SSH_MOUNT_CONTAINER} bash -c $"echo \n >> ${MOUNT_PATH}/.ssh/authorized_keys"
docker exec -it ${SSH_MOUNT_CONTAINER} bash -c "echo ${MOUNT_SSH_PUB_KEY_CONTENT} >> ${MOUNT_PATH}/.ssh/authorized_keys"

echo "------------ test 1 identity_file flag ------------\n"

# test1: ssh key
docker plugin disable ${SSH_MOUNT_PLUGIN}
docker plugin set ${SSH_MOUNT_PLUGIN} sshkey.source=${TEST_SSH_KEY_DIRECTORY}
docker plugin enable ${SSH_MOUNT_PLUGIN}
docker volume create -d ${SSH_MOUNT_PLUGIN} -o identity_file=/root/.ssh/id_rsa -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o port=${MOUNT_PORT} ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

echo "------------ test 2 id_rsa flag ------------\n"

# test2: ssh id_rsa flag
docker volume create -d ${SSH_MOUNT_PLUGIN} -o sshcmd=${MOUNT_USER}@${MOUNT_HOST}:${MOUNT_PATH} -o port=${MOUNT_PORT} -o id_rsa="$(cat $TEST_SSH_KEY_PATH)" ${SSH_TEST_VOLUME}
docker run --rm -v ${SSH_TEST_VOLUME}:/write busybox sh -c "echo hello > /write/world"
docker run --rm -v ${SSH_TEST_VOLUME}:/read busybox grep -Fxq hello /read/world
docker volume rm ${SSH_TEST_VOLUME}

# Cleanup
make testclean clean TAG=${TAG} TEST_SSH_MOUNT_CONTAINER=${SSH_MOUNT_CONTAINER} TEST_SSH_VOLUME=${SSH_TEST_VOLUME}