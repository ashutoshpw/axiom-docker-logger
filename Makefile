PLUGIN_NAME ?= ashutoshpw/axiom-docker-logger
PLUGIN_TAG ?= latest
BUILDER_IMAGE ?= axiom-logger-builder

all: clean build create enable

clean:
	@echo "Cleaning up..."
	-docker plugin disable $(PLUGIN_NAME):$(PLUGIN_TAG) 2>/dev/null || true
	-docker plugin rm $(PLUGIN_NAME):$(PLUGIN_TAG) 2>/dev/null || true
	rm -rf ./plugin/rootfs

build:
	@echo "Building binary..."
	docker build -t $(BUILDER_IMAGE) .
	@echo "Extracting rootfs..."
	mkdir -p ./plugin/rootfs
	docker create --name axiom-tmp $(BUILDER_IMAGE)
	docker export axiom-tmp | tar -x -C ./plugin/rootfs
	docker rm axiom-tmp
	docker rmi $(BUILDER_IMAGE)

create:
	@echo "Creating plugin $(PLUGIN_NAME):$(PLUGIN_TAG)..."
	docker plugin create $(PLUGIN_NAME):$(PLUGIN_TAG) ./plugin

enable:
	@echo "Enabling plugin..."
	docker plugin enable $(PLUGIN_NAME):$(PLUGIN_TAG)

rm:
	@echo "Removing plugin $(PLUGIN_NAME):$(PLUGIN_TAG)..."
	-docker plugin rm -f $(PLUGIN_NAME):$(PLUGIN_TAG) 2>/dev/null || true

push:
	@echo "Pushing plugin $(PLUGIN_NAME):$(PLUGIN_TAG) to registry..."
	docker plugin push $(PLUGIN_NAME):$(PLUGIN_TAG)

smoke: create
	@echo "Smoke testing plugin..."
	docker plugin set $(PLUGIN_NAME):$(PLUGIN_TAG) AXIOM_TOKEN=xaat-ci-dummy
	docker plugin enable $(PLUGIN_NAME):$(PLUGIN_TAG)
	docker plugin inspect $(PLUGIN_NAME):$(PLUGIN_TAG) >/dev/null
	docker plugin disable $(PLUGIN_NAME):$(PLUGIN_TAG)
	docker plugin rm $(PLUGIN_NAME):$(PLUGIN_TAG)

# For versioned releases
release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required: make release VERSION=1.2.3"; exit 1; fi
	$(MAKE) clean build
	$(MAKE) PLUGIN_TAG=$(VERSION) smoke
	./scripts/publish.sh "$(VERSION)" latest
