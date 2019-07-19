VERSION := latest
DOCKER_IMAGE_NAME := containous/i3o

BINARY_NAME = i3o
DIST_DIR = $(CURDIR)/dist
DIST_DIR_I3O = $(DIST_DIR)/$(BINARY_NAME)
PROJECT ?= github.com/containous/$(BINARY_NAME)
GOLANGCI_LINTER_VERSION = v1.17.1

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

INTEGRATION_TEST_OPTS := -timeout 30m

DOCKER_INTEGRATION_TEST_NAME := $(DOCKER_IMAGE_NAME)-integration-tests
DOCKER_INTEGRATION_TEST_OTPS := -v $(CURDIR):/i3o --privileged -e INTEGRATION_TEST_OPTS

export GO111MODULE=on

default: clean check test build

$(DIST_DIR):
	mkdir -p $(DIST_DIR)

clean:
	rm -rf dist/ cover.out

# Static linting of source files. See .golangci.toml for options
local-check: $(DIST_DIR)
	golangci-lint run --config .golangci.toml

# Build
local-build: $(DIST_DIR)
	CGO_ENABLED=0 go build -o ${DIST_DIR_I3O} -ldflags="-s -w \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.version=$(VERSION) \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.commit=$(SHA) \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.date=$(BUILD_DATE)" \
	$(CURDIR)/cmd/$(BINARY_NAME)/*.go

local-test: clean
	go test -v -cover ./...

# Integration test
local-test-integration: $(DIST_DIR) kubectl helm build
	CGO_ENABLED=0 go test ./integration -integration $(INTEGRATION_TEST_OPTS) -check.v -v

test-integration:
	docker build -t $(DOCKER_INTEGRATION_TEST_NAME) integration/resources/build
	docker run --rm $(DOCKER_INTEGRATION_TEST_OTPS) $(DOCKER_INTEGRATION_TEST_NAME) make local-test-integration

kubectl:
	@command -v kubectl >/dev/null 2>&1 || (curl -LO https://storage.googleapis.com/kubernetes-release/release/$(shell curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x ./kubectl && sudo mv ./kubectl /usr/local/bin/kubectl)

helm:
	@command -v helm >/dev/null 2>&1 || curl https://raw.githubusercontent.com/helm/helm/v2.14.1/scripts/get | bash

build: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):latest" --build-arg="MAKE_TARGET=local-build" $(CURDIR)/
	docker run --name=build -t "$(DOCKER_IMAGE_NAME):latest" version
	docker cp build:/app/$(BINARY_NAME) $(DIST_DIR)/
	docker rm build

test: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):test" --target maker --build-arg="MAKE_TARGET=local-test" $(CURDIR)/

check: $(DIST_DIR)
	docker run -t --rm -v $(CURDIR):/go/src/$(PROJECT) -w /go/src/$(PROJECT) -e GO111MODULE golangci/golangci-lint:$(GOLANGCI_LINTER_VERSION) golangci-lint run --config .golangci.toml

push-docker: build
	docker tag "$(DOCKER_IMAGE_NAME):latest" ${DOCKER_IMAGE_NAME}:${VERSION}
	docker push ${DOCKER_IMAGE_NAME}:${VERSION}
	docker push $(DOCKER_IMAGE_NAME):latest

# Update vendor directory
vendor:
	go mod vendor

upgrade:
	go get -u
	go mod tidy

tidy:
	go mod tidy

helm-lint:
	helm lint helm/chart/i3o

.PHONY: local-check local-build local-test check build test push-docker \
		vendor helm-lint helm kubectl test-integration local-test-integration
