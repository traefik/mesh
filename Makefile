DOCKER_IMAGE_NAME := containous/maesh
UNAME := $(shell uname)
SRCS = $(shell git ls-files '*.go' | grep -v '^vendor/')

BINARY_NAME = maesh
DIST_DIR = $(CURDIR)/dist
DIST_DIR_MAESH = $(DIST_DIR)/$(BINARY_NAME)
PROJECT ?= github.com/containous/$(BINARY_NAME)

TAG_NAME ?= $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

INTEGRATION_TEST_OPTS := -test.timeout=20m -check.vv -v

export GO111MODULE=on

default: clean check test build

$(DIST_DIR):
	mkdir -p $(DIST_DIR)

clean:
	rm -rf $(CURDIR)/dist/ cover.out $(CURDIR)/pages $(CURDIR)/gh-pages.zip $(CURDIR)/maesh-gh-pages

# Static linting of source files. See .golangci.toml for options
local-check: $(DIST_DIR) helm-lint
	golangci-lint run --config .golangci.toml

# Local commands
local-build: $(DIST_DIR)
	CGO_ENABLED=0 go build -o ${DIST_DIR_MAESH} -ldflags="-s -w \
	-X github.com/containous/$(BINARY_NAME)/pkg/version.Version=$(VERSION) \
	-X github.com/containous/$(BINARY_NAME)/pkg/version.Commit=$(SHA) \
	-X github.com/containous/$(BINARY_NAME)/pkg/version.Date=$(BUILD_DATE)" \
	$(CURDIR)/cmd/$(BINARY_NAME)/*.go

local-test: clean
	go test -v -cover ./...

ifeq ($(UNAME), Linux)
test-integration: $(DIST_DIR) kubectl helm build k3d
else
test-integration: $(DIST_DIR) kubectl helm build local-build k3d
endif
	CGO_ENABLED=0 go test ./integration -integration $(INTEGRATION_TEST_OPTS) $(TESTFLAGS)

test-integration-nobuild: $(DIST_DIR) kubectl helm k3d
	CGO_ENABLED=0 go test ./integration -integration $(INTEGRATION_TEST_OPTS) $(TESTFLAGS)

kubectl:
	@command -v kubectl >/dev/null 2>&1 || (curl -LO https://storage.googleapis.com/kubernetes-release/release/$(shell curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x ./kubectl && sudo mv ./kubectl /usr/local/bin/kubectl)

build: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):latest" --build-arg="MAKE_TARGET=local-build" $(CURDIR)/
	docker run --name=build -t "$(DOCKER_IMAGE_NAME):latest" version
	docker cp build:/app/$(BINARY_NAME) $(DIST_DIR)/
	docker rm build

test: $(DIST_DIR)
	docker build --tag "$(DOCKER_IMAGE_NAME):test" --target maker --build-arg="MAKE_TARGET=local-test" $(CURDIR)/

check: $(DIST_DIR) helm-lint
	docker build --tag "$(DOCKER_IMAGE_NAME):check" --target base-image $(CURDIR)/
	docker run --rm \
      -v $(CURDIR):/go/src/$(PROJECT) \
      -w /go/src/$(PROJECT) \
      -e GO111MODULE \
      "$(DOCKER_IMAGE_NAME):check" golangci-lint run --config .golangci.toml

publish-images: build
	seihon publish -v "$(VERSION)" -v "latest" --image-name ${DOCKER_IMAGE_NAME} --dry-run=false --base-runtime-image=alpine:3.10

## Create packages for the release
release-packages: vendor build
	rm -rf dist
	docker build --tag "$(DOCKER_IMAGE_NAME):release-packages" --target base-image $(CURDIR)/
	docker run --rm \
      -v $(CURDIR):/go/src/$(PROJECT) \
      -w /go/src/$(PROJECT) \
      -e GITHUB_TOKEN \
      "$(DOCKER_IMAGE_NAME):release-packages" goreleaser release --skip-publish
	docker run --rm \
	  -v $(CURDIR):/go/src/$(PROJECT) \
	  -w /go/src/$(PROJECT) \
	  "$(DOCKER_IMAGE_NAME):release-packages" chown -R $(shell id -u):$(shell id -g) dist/

## Format the Code
fmt:
	gofmt -s -l -w $(SRCS)

## Update vendor directory
vendor:
	go mod vendor

upgrade:
	go get -u
	go mod tidy

tidy:
	go mod tidy

helm:
	@command -v helm >/dev/null 2>&1 || curl -L https://git.io/get_helm.sh | bash -s -- -v v3.0.1

helm-lint: helm
	helm lint helm/chart/maesh

k3d:
	@command -v k3d >/dev/null 2>&1 || curl -s https://raw.githubusercontent.com/rancher/k3d/v1.5.1/install.sh | TAG=v1.5.1 bash

pages:
	mkdir -p $(CURDIR)/pages
	rm -rf $(CURDIR)/gh-pages.zip $(CURDIR)/maesh-gh-pages
	curl -sSLO https://$(PROJECT)/archive/gh-pages.zip
	unzip $(CURDIR)/gh-pages.zip
	# We only keep the directory "charts" so documentation may remove files
	cp -r $(CURDIR)/maesh-gh-pages/charts $(CURDIR)/pages/
	rm -rf $(CURDIR)/gh-pages.zip $(CURDIR)/maesh-gh-pages

docs-package: pages
	make -C $(CURDIR)/docs
	cp -r $(CURDIR)/docs/site/* $(CURDIR)/pages/
	cp $(CURDIR)/docs/CNAME $(CURDIR)/pages/CNAME

helm-package: helm-lint pages
	helm package --app-version $(TAG_NAME) $(CURDIR)/helm/chart/maesh
	cp helm/chart/maesh/README.md index.md
	mkdir -p $(CURDIR)/pages/charts
	mv *.tgz index.md $(CURDIR)/pages/charts/
	helm repo index $(CURDIR)/pages/charts/

.PHONY: local-check local-build local-test check build test publish-images \
		vendor kubectl test-integration local-test-integration pages \
		helm helm-lint helm-package k3d
