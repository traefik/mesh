FROM golang:1.16-alpine AS base-image

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

WORKDIR /go/src/github.com/traefik/mesh

# Download goreleaser binary to bin folder in $GOPATH
RUN curl -sfL https://gist.githubusercontent.com/traefiker/6d7ac019c11d011e4f131bb2cca8900e/raw/goreleaser.sh | sh

# Download golangci-lint binary to bin folder in $GOPATH
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $GOPATH/bin v1.32.0

ENV GO111MODULE on
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM base-image as maker

ARG MAKE_TARGET=local-build

RUN make ${MAKE_TARGET}

## IMAGE
FROM alpine:3.10

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

COPY --from=base-image /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=maker /go/src/github.com/traefik/mesh/dist/traefik-mesh /app/

ENTRYPOINT ["/app/traefik-mesh"]
