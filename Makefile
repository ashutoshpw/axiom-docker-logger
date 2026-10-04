PLUGIN_NAME ?= ashutoshpw/axiom-docker-logger
PLUGIN_TAG ?= latest
PLUGIN_DIR ?= plugin
GO ?= go

.PHONY: all build create enable push smoke test clean
all: build
build:
	PLUGIN_DIR=$(PLUGIN_DIR) ./scripts/build-rootfs.sh
create:
	docker plugin create $(PLUGIN_NAME):$(PLUGIN_TAG) $(PLUGIN_DIR)
enable:
	docker plugin enable $(PLUGIN_NAME):$(PLUGIN_TAG)
push:
	docker plugin push $(PLUGIN_NAME):$(PLUGIN_TAG)
smoke:
	GO=$(GO) ./scripts/smoke.sh
test:
	$(GO) test -race ./...
	$(GO) vet ./...
clean:
	rm -rf ./plugin/rootfs
