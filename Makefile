NAME=ucphhpc/sshfs
TAG?=latest
BUILD_ARGS=
TEST_SSH_MOUNT_CONTAINER=ssh-mount-dummy
TEST_SSH_VOLUME=ssh-test-volume

all: clean rootfs build create

clean:
	@echo "### rm ./plugin"
	@rm -rf ./plugin

rootfs:
	@echo "### docker build: rootfs image with docker-volume-sshfs"
	@docker build -q -t ${NAME}:rootfs .
	@echo "### create rootfs directory in ./plugin/rootfs"
	@mkdir -p ./plugin/rootfs
	@docker create --name tmp ${NAME}:rootfs
	@docker export tmp | tar -x -C ./plugin/rootfs
	@echo "### copy config.json to ./plugin/"
	@cp config.json ./plugin/
	@docker rm -vf tmp

build:
	@docker build -q -t ${NAME}:${TAG} ${BUILD_ARGS} .

create:
	@echo "### remove existing plugin ${NAME}:${TAG} if exists"
	@docker plugin rm -f ${NAME}:${TAG} || true
	@echo "### create new plugin ${NAME}:${TAG} from ./plugin"
	@docker plugin create ${NAME}:${TAG} ./plugin

enable:
	@echo "### enable plugin ${NAME}:${TAG}"
	@docker plugin enable ${NAME}:${TAG}

disable:
	@echo "### disable plugin ${NAME}:${TAG}"
	@docker plugin disable ${NAME}:${TAG}

# https://github.com/docker/buildx/issues/1513
push: clean rootfs create enable
	@echo "### push plugin ${NAME}:${TAG}"
	@docker plugin push ${NAME}:${TAG}

uninstalltest:
### PLACEHOLDER (it's purpose is to uninstall depedencies for test) ###

installtest:
### PLACEHOLDER (this will install the dependencies for test) ###

test: override TAG=test
test:
	@.gocd/integration.sh
	@.gocd/key-auth.sh

testclean: override TAG=test
testclean:
	@docker stop ${TEST_SSH_MOUNT_CONTAINER} > /dev/null 2>&1 || echo 0 > /dev/null
	@docker rm ${TEST_SSH_MOUNT_CONTAINER} --force > /dev/null 2>&1 || echo 0 > /dev/null 
	@docker volume rm ${TEST_SSH_VOLUME} --force > /dev/null 2>&1 || echo 0 > /dev/null
	@docker plugin disable ${NAME}:${TAG} --force > /dev/null 2>&1 || echo 0 > /dev/null
	@docker plugin rm ${NAME}:${TAG} --force > /dev/null 2>&1 || echo 0 > /dev/null
