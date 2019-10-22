DOCKER_IMAGE_NAME := containous/maesh

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

DOCKER_INTEGRATION_TEST_NAME := $(DOCKER_IMAGE_NAME)-integration-tests
DOCKER_INTEGRATION_TEST_OTPS := -v $(CURDIR):/maesh --privileged -e INTEGRATION_TEST_OPTS

export GO111MODULE=on

default: clean check test build

$(DIST_DIR):
	mkdir -p $(DIST_DIR)

clean:
	rm -rf $(CURDIR)/dist/ cover.out $(CURDIR)/pages $(CURDIR)/gh-pages.zip $(CURDIR)/maesh-gh-pages

# Static linting of source files. See .golangci.toml for options
local-check: $(DIST_DIR) helm-lint
	golangci-lint run --config .golangci.toml

# Build
local-build: $(DIST_DIR)
	CGO_ENABLED=0 go build -o ${DIST_DIR_MAESH} -ldflags="-s -w \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.version=$(VERSION) \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.commit=$(SHA) \
	-X github.com/containous/$(BINARY_NAME)/cmd/version.date=$(BUILD_DATE)" \
	$(CURDIR)/cmd/$(BINARY_NAME)/*.go

local-test: clean
	go test -v -cover ./...

# Integration test
local-test-integration: $(DIST_DIR) kubectl helm build
	CGO_ENABLED=0 go test ./integration -integration $(INTEGRATION_TEST_OPTS)

test-integration:
	docker build -t $(DOCKER_INTEGRATION_TEST_NAME) integration/resources/build
	docker run --rm $(DOCKER_INTEGRATION_TEST_OTPS) $(DOCKER_INTEGRATION_TEST_NAME) make local-test-integration

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
	@command -v helm >/dev/null 2>&1 || curl -L https://git.io/get_helm.sh | bash -s -- -v v2.14.3
	@helm init --client-only

helm-lint: helm
	helm lint helm/chart/maesh

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
	cp helm/chart/README.md index.md
	mkdir -p $(CURDIR)/pages/charts
	mv *.tgz index.md $(CURDIR)/pages/charts/
	helm repo index $(CURDIR)/pages/charts/

.PHONY: local-check local-build local-test check build test publish-images \
		vendor kubectl test-integration local-test-integration pages
.PHONY: helm helm-lint helm-package
