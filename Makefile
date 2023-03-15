PLUGIN_NAME=ucphhpc/sshfs
PLUGIN_TAG?=latest
PLATFORMS=linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v8

all: clean rootfs create

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

create:
	@echo "### remove existing plugin ${PLUGIN_NAME}:${PLUGIN_TAG} if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	@echo "### create new plugin ${PLUGIN_NAME}:${PLUGIN_TAG} from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./plugin

enable:		
	@echo "### enable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"		
	@docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

# https://github.com/docker/buildx/issues/1513
push:  clean rootfs create enable
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker buildx build --push --platform $(PLATFORMS) -t ${PLUGIN_NAME}:${PLUGIN_TAG} --provenance false .

uninstallcheck:
### PLACEHOLDER (it's purpose is to uninstall depedencies for check) ###

installcheck:
### PLACEHOLDER (this will install the dependencies for check) ###

check:
	@.gocd/integration.sh
	@.gocd/key-auth.sh
