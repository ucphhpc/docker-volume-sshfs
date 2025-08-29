PLUGIN_NAME=ucphhpc/sshfs
PLUGIN_TAG?=latest
PLATFORM=linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v8
TEST_SSH_MOUNT_CONTAINER=ssh-mount-dummy
TEST_SSH_VOLUME=ssh-test-volume

all: clean rootfs build create

clean:
	@echo "### rm ./plugin"
	@rm -rf ./plugin

rootfs:
	@echo "### docker build: rootfs image with docker-volume-sshfs"
	@docker build -q -t ${PLUGIN_NAME}:rootfs .
	@echo "### create rootfs directory in ./plugin/rootfs"
	@mkdir -p ./plugin/rootfs
	@docker create --name tmp ${PLUGIN_NAME}:rootfs
	@docker export tmp | tar -x -C ./plugin/rootfs
	@echo "### copy config.json to ./plugin/"
	@cp config.json ./plugin/
	@docker rm -vf tmp

build:
	@docker build --platform ${PLATFORM} -q -t ${PLUGIN_NAME}:${PLUGIN_TAG} .

create:
	@echo "### remove existing plugin ${PLUGIN_NAME}:${PLUGIN_TAG} if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	@echo "### create new plugin ${PLUGIN_NAME}:${PLUGIN_TAG} from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./plugin

enable:
	@echo "### enable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"		
	@docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

# https://github.com/docker/buildx/issues/1513
push: clean rootfs create enable
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}

uninstalltest:
### PLACEHOLDER (it's purpose is to uninstall depedencies for test) ###

installtest:
### PLACEHOLDER (this will install the dependencies for test) ###

test: override PLUGIN_TAG=test
test:
	@.gocd/integration.sh
	@.gocd/key-auth.sh

testclean: override PLUGIN_TAG=test
testclean:
	@docker stop ${TEST_SSH_MOUNT_CONTAINER} > /dev/null 2>&1 || echo 0 > /dev/null
	@docker rm ${TEST_SSH_MOUNT_CONTAINER} --force > /dev/null 2>&1 || echo 0 > /dev/null 
	@docker volume rm ${TEST_SSH_VOLUME} --force > /dev/null 2>&1 || echo 0 > /dev/null
	@docker plugin disable ${PLUGIN_NAME}:${PLUGIN_TAG} --force > /dev/null 2>&1 || echo 0 > /dev/null
	@docker plugin rm ${PLUGIN_NAME}:${PLUGIN_TAG} --force > /dev/null 2>&1 || echo 0 > /dev/null
