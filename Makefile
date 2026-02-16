PLUGIN_NAME=ashutoshpw/axiom-docker-logger
PLUGIN_TAG=latest

all: clean build create enable

clean:
	@echo "Cleaning up..."
	-docker plugin disable $(PLUGIN_NAME):$(PLUGIN_TAG) 2>/dev/null || true
	-docker plugin rm $(PLUGIN_NAME):$(PLUGIN_TAG) 2>/dev/null || true
	rm -rf ./plugin/rootfs

build:
	@echo "Building binary..."
	docker build -t axiom-logger-builder .
	@echo "Extracting rootfs..."
	mkdir -p ./plugin/rootfs
	docker create --name axiom-tmp axiom-logger-builder
	docker export axiom-tmp | tar -x -C ./plugin/rootfs
	docker rm axiom-tmp
	docker rmi axiom-logger-builder

create:
	@echo "Creating plugin..."
	docker plugin create $(PLUGIN_NAME):$(PLUGIN_TAG) ./plugin

enable:
	@echo "Enabling plugin..."
	docker plugin enable $(PLUGIN_NAME):$(PLUGIN_TAG)

push:
	@echo "Pushing plugin to registry..."
	docker plugin push $(PLUGIN_NAME):$(PLUGIN_TAG)

# For versioned releases
release:
	$(MAKE) PLUGIN_TAG=$(VERSION) clean build create push
