VERSION := latest
DOCKER_IMAGE_NAME := containous/i3o

BINARY_NAME = i3o
DIST_DIR = $(CURDIR)/dist
DIST_DIR_I3O = $(DIST_DIR)/$(BINARY_NAME)
PROJECT ?= github.com/containous/$(BINARY_NAME)
GOLANGCI_LINTER_VERSION = v1.16.0

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
local-test-integration: $(DIST_DIR) local-build
	CGO_ENABLED=0 go test ./integration -integration "-test.timeout=20m" -check.v

build: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):latest" --build-arg="MAKE_TARGET=local-build" $(CURDIR)/
	docker run --name=build -t "$(DOCKER_IMAGE_NAME):latest" version
	docker cp build:/app/$(BINARY_NAME) $(DIST_DIR)/
	docker rm build

check: $(DIST_DIR)
	docker run -t --rm -v $(CURDIR):/go/src/$(PROJECT) -w /go/src/$(PROJECT) -e GO111MODULE golangci/golangci-lint:$(GOLANGCI_LINTER_VERSION) golangci-lint run --config .golangci.toml

# Build docker image
build-docker: build
	docker build -f ./Dockerfile -t ${DOCKER_IMAGE_NAME}:${VERSION} .

push-docker:
	docker push ${DOCKER_IMAGE_NAME}:${VERSION}

# Update vendor directory
vendor:
	go mod vendor

helm-lint:
	helm lint helm/chart/i3o

.PHONY: local-check local-build check build build-docker push-docker vendor helm-lint
