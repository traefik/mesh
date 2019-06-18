VERSION := latest
DOCKER_IMAGE_NAME := containous/i3o

BINARY_NAME = i3o
DIST_DIR = $(CURDIR)/dist
DIST_DIR_I3O = $(DIST_DIR)/$(BINARY_NAME)
PROJECT ?= github.com/containous/$(BINARY_NAME)
GOLANGCI_LINTER_VERSION = v1.16.0

INTEGRATION_TEST_OPTS := -timeout 20m

export GO111MODULE=on

default: check build

$(DIST_DIR):
	mkdir -p $(DIST_DIR)

# Static linting of source files. See .golangci.toml for options
local-check: $(DIST_DIR)
	golangci-lint run --config .golangci.toml

# Build
local-build: $(DIST_DIR)
	CGO_ENABLED=0 go build -o ${DIST_DIR_I3O} ./

# Integration test
test-integration: $(DIST_DIR) kubectl helm build
	CGO_ENABLED=0 go test ./integration -integration $(INTEGRATION_TEST_OPTS) -check.v

kubectl:
	@command -v kubectl >/dev/null 2>&1 || (curl -LO https://storage.googleapis.com/kubernetes-release/release/$(shell curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x ./kubectl && sudo mv ./kubectl /usr/local/bin/kubectl)

helm:
	@command -v helm >/dev/null 2>&1 || curl https://raw.githubusercontent.com/helm/helm/v2.14.1/scripts/get | bash

build: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):latest" --build-arg="MAKE_TARGET=local-build" $(CURDIR)/
	docker run --name=build -t "$(DOCKER_IMAGE_NAME):latest" version
	docker cp build:/app/$(BINARY_NAME) $(DIST_DIR)/
	docker rm build

check: $(DIST_DIR)
	docker run -t --rm -v $(CURDIR):/go/src/$(PROJECT) -w /go/src/$(PROJECT) -e GO111MODULE golangci/golangci-lint:$(GOLANGCI_LINTER_VERSION) golangci-lint run --config .golangci.toml

push-docker: build
	docker tag "$(DOCKER_IMAGE_NAME):latest" ${DOCKER_IMAGE_NAME}:${VERSION}
	docker push ${DOCKER_IMAGE_NAME}:${VERSION}

# Update vendor directory
vendor:
	go mod vendor

helm-lint:
	helm lint helm/chart/i3o

.PHONY: local-check local-build check build build-docker push-docker vendor helm-lint helm kubectl
