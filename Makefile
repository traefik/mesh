default: build

build:
	go build -o i3o *.go 

# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

.PHONY: check build
