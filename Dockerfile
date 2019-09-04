# Let's build maesh for linux-amd64
FROM golang:1.13-alpine AS base-image

# Package dependencies
RUN apk --no-cache --no-progress add \
    bash \
    gcc \
    git \
    make \
    musl-dev \
    mercurial \
    curl \
    tar \
    ca-certificates \
    tzdata \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

ENV PROJECT_WORKING_DIR=/go/src/github.com/containous/maesh

# Download goreleaser binary to bin folder in $GOPATH
RUN curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sh

WORKDIR "${PROJECT_WORKING_DIR}"
COPY go.mod go.sum "${PROJECT_WORKING_DIR}"/
RUN GO111MODULE=on GOPROXY=https://proxy.golang.org go mod download
COPY . "${PROJECT_WORKING_DIR}/"

FROM base-image as maker

ARG MAKE_TARGET=local-build

RUN make ${MAKE_TARGET}

## IMAGE
FROM alpine:3.10

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

COPY --from=maker /go/src/github.com/containous/maesh/dist/maesh /app/

ENTRYPOINT ["/app/maesh"]
