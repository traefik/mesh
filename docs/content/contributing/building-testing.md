---
title: "Traefik Mesh Building and Testing"
description: "This documentation article explains how you can build your own Traefik Mesh binary from the sources."
---

# Building and Testing

So you want to build your own Traefik Mesh binary from the sources? Let's see how.

## Building

To build Traefik Mesh from the sources you need either [Docker](https://github.com/docker/docker) and [make](https://www.gnu.org/software/make/manual/make.html), 
or [Go](https://github.com/golang/go). 

### With `Docker` and `make`

Traefik Mesh can be built from the sources by using the `make` command.
This will create a binary for the Linux platform in the `dist` directory and a Docker image:

```bash
$ make
#[...]
Successfully tagged traefik/mesh:latest
docker run --name=build -t "traefik/mesh:latest" version
version:
 version     : 019a39d
 commit      : 019a39d
 build date  : 2023-10-14_02:17:49PM
 go version  : go1.21.3
 go compiler : gc
 platform    : linux/amd64
#[...]

$ ls dist/
traefik-mesh
``` 

!!! Note
    The default `make` target invokes the `clean`, `check`, `test` and `build` targets.

### With `Go`

Requirements:

- `Go` v1.19+
- Environment variable `GO111MODULE=on`

One your Go environment is set up, you can build Traefik Mesh from the sources by using the `go build` command.
The Go compiler will build an executable for your platform.

```bash
$ go build -o dist/traefik-mesh cmd/mesh/mesh.go
$ ./dist/traefik-mesh version
version:
 version     : dev
 commit      : I don't remember exactly
 build date  : I don't remember exactly
 go version  : go1.21
 go compiler : gc
 platform    : linux/amd64
```

## Testing

### With `Docker` and `make`

Run unit tests by using the `test` target:

```bash
$ make test
docker build --tag "traefik/mesh:test" --target maker --build-arg="MAKE_TARGET=local-test" /home/user/traefik-mesh/
#[...]
--- PASS: TestBuildConfiguration (0.00s)
    --- PASS: TestBuildConfiguration/simple_configuration_build_with_HTTP_service (0.20s)
PASS
coverage: 69.7% of statements
ok  	github.com/traefik/mesh/pkg/providers/smi	1.982s	coverage: 69.7% of statements
?   	github.com/traefik/mesh/pkg/signals	[no test files]
Removing intermediate container 4e887c16ddee
 ---> 75d44229a46e
Successfully built 75d44229a46e
Successfully tagged traefik/mesh:test
```

Run the integration tests by using the `test-integration` target. For development purposes, you can specify which tests 
to run by using the `TESTFLAGS` environment variable (only works with the `test-integration` target):

```bash
# Run every tests in the MyTest suite
$ TESTFLAGS="-check.f MyTestSuite" make test-integration

# Run the test "MyTest" in the MyTest suite
$ TESTFLAGS="-check.f MyTestSuite.MyTest" make test-integration

# Run every tests starting with "My", in the MyTest suite
$ TESTFLAGS="-check.f MyTestSuite.My" make test-integration

# Run every tests ending with "Test", in the MyTest suite
$ TESTFLAGS="-check.f MyTestSuite.*Test" make test-integration
```

More on [https://labix.org/gocheck](https://labix.org/gocheck).

### With `Go`

Run the unit tests by using the `go test` command:

```bash
$ go test -v ./...
#[...]
=== RUN   TestGroupTrafficTargetsByDestination
--- PASS: TestGroupTrafficTargetsByDestination (0.20s)
=== RUN   TestBuildConfiguration
=== RUN   TestBuildConfiguration/simple_configuration_build_with_HTTP_service
=== PAUSE TestBuildConfiguration/simple_configuration_build_with_HTTP_service
=== CONT  TestBuildConfiguration/simple_configuration_build_with_HTTP_service
time="2020-04-09T16:09:16+04:00" level=debug msg="Found traffictargets for service default/demo-service: [0xc0009004e0]"
time="2020-04-09T16:09:16+04:00" level=debug msg="Found applicable traffictargets for service default/demo-service: [0xc0009004e0]"
time="2020-04-09T16:09:16+04:00" level=debug msg="Found grouped traffictargets for service default/demo-service: map[{name:api-service namespace:default port:}:[0xc000900820]]"
time="2020-04-09T16:09:16+04:00" level=debug msg="No TrafficSplits in namespace: default"
time="2020-04-09T16:09:16+04:00" level=debug msg="Found trafficsplits for service default/demo-service: []"
--- PASS: TestBuildConfiguration (0.00s)
    --- PASS: TestBuildConfiguration/simple_configuration_build_with_HTTP_service (0.21s)
PASS
ok  	github.com/traefik/mesh/pkg/providers/smi	3.634s
?   	github.com/traefik/mesh/pkg/signals	[no test files]
```

Run the integration tests in the `integration` directory by using the `go test ./integration -integration` command:

```bash
$ go test -v ./integration -integration -check.f CoreDNSSuite
#[...]
OK: 2 passed
--- PASS: Test (161.20s)
PASS
ok  	github.com/traefik/mesh/integration	162.695s
```

!!! Important
    Before running the integration tests, build the Traefik Mesh Docker image.
    Check out the [Building](#building) section for more details.
