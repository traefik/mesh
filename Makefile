default: build

build:
	go build -o traefik-mesh-controller *.go 

local-validate: local-validate-lint

local-validate-lint:
	golangci-lint run
