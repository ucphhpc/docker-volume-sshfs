#!/bin/bash

set -e
set -x

TAG=test

# install
sudo docker pull ucphhpc/ssh-mount-dummy
sudo docker pull busybox

# make the plugin
sudo PLUGIN_TAG=$TAG make
# enable the plugin
sudo docker plugin enable ucphhpc/sshfs:$TAG
# list plugins
sudo docker plugin ls
# start sshd
sudo docker run -d -p 2222:22 sshd

# test1: simple
sudo docker volume create -d ucphhpc/sshfs:$TAG -o sshcmd=root@localhost:/ -o port=2222 -o password=root sshvolume
sudo docker run --rm -v sshvolume:/write busybox sh -c "echo hello > /write/world"
sudo docker run --rm -v sshvolume:/read busybox grep -Fxq hello /read/world
#sudo cat /var/lib/docker/plugins/sshfs-state.json
sudo docker volume rm sshvolume

# test2: allow_other
sudo docker volume create -d ucphhpc/sshfs:$TAG -o sshcmd=root@localhost:/ -o allow_other -o port=2222 -o password=root sshvolume
sudo docker run --rm -v sshvolume:/write -u nobody busybox sh -c "echo hello > /write/world"
docker run --rm -v sshvolume:/read -u nobody busybox grep -Fxq hello /read/world
#sudo cat /var/lib/docker/plugins/sshfs-state.json
sudo docker volume rm sshvolume

# test3: compression
sudo docker volume create -d ucphhpc/sshfs:$TAG -o sshcmd=root@localhost:/ -o Ciphers=chacha20-poly1305@openssh.com -o Compression=no -o port=2222 -o password=root sshvolume
sudo docker run --rm -v sshvolume:/write busybox sh -c "echo hello > /write/world"
sudo docker run --rm -v sshvolume:/read busybox grep -Fxq hello /read/world
#sudo cat /var/lib/docker/plugins/sshfs-state.json
sudo docker volume rm sshvolume

# test4: source
sudo docker plugin disable ucphhpc/sshfs:$TAG
sudo docker plugin set ucphhpc/sshfs:$TAG state.source=/tmp
sudo docker plugin enable ucphhpc/sshfs:$TAG
sudo docker volume create -d ucphhpc/sshfs:$TAG -o sshcmd=root@localhost:/ -o Ciphers=chacha20-poly1305@openssh.com -o Compression=no -o port=2222 -o password=root sshvolume
sudo docker run --rm -v sshvolume:/write busybox sh -c "echo hello > /write/world"
sudo docker run --rm -v sshvolume:/read busybox grep -Fxq hello /read/world
#sudo cat /tmp/sshfs-state.json
sudo docker volume rm sshvolume

# test5: ssh key
sudo docker plugin disable ucphhpc/sshfs:$TAG
sudo docker plugin set ucphhpc/sshfs:$TAG sshdir.source=`pwd`/.travis/ssh
sudo docker plugin enable ucphhpc/sshfs:$TAG
sudo docker volume create -d ucphhpc/sshfs:$TAG -o IdentityFile=/root/.ssh/id_rsa -o sshcmd=root@localhost:/ -o port=2222 sshvolume
sudo docker run --rm -v sshvolume:/write busybox sh -c "echo hello > /write/world"
sudo docker run --rm -v sshvolume:/read busybox grep -Fxq hello /read/world
#sudo cat /var/lib/docker/plugins/sshfs-state.json
sudo docker volume rm sshvolume

# test6: ssh id_rsa flag
sudo docker volume create -d ucphhpc/sshfs:$TAG -o sshcmd=root@localhost:/ -o port=2222 -o id_rsa="$(cat `pwd`/.travis/ssh/id_rsa)" sshvolume
sudo docker run --rm -v sshvolume:/write busybox sh -c "echo hello > /write/world"
sudo docker run --rm -v sshvolume:/read busybox grep -Fxq hello /read/world
#sudo cat /var/lib/docker/plugins/sshfs-state.json
sudo docker volume rm sshvolume
