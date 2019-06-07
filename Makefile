VERSION := v0.0.0-alpha1
DOCKER_TAG := containous/i3o

# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

# Build
build:
	CGO_ENABLED=0 go build -o dist/i3o ./

# Build docker image
build-docker: build
	docker build -f ./Dockerfile -t ${DOCKER_TAG}:${VERSION} .

.PHONY: check build build-docker
