default: build

build:
	go build -o traefik-mesh-controller *.go 

# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

.PHONY: check build
