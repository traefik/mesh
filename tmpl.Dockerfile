FROM golang:1.21-alpine AS builder

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
    xz \
    ca-certificates \
    tzdata \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

WORKDIR /go/src/github.com/traefik/mesh

# Download goreleaser binary to bin folder in $GOPATH
RUN curl -sfL https://gist.githubusercontent.com/traefiker/6d7ac019c11d011e4f131bb2cca8900e/raw/goreleaser.sh | sh

ENV GO111MODULE on
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GOARCH={{ .GoARCH }} GOARM={{ .GoARM }} make local-build
RUN curl -sSfL https://github.com/upx/upx/releases/download/v4.1.0/upx-4.1.0-amd64_linux.tar.xz | tar xJvf - --strip-components 1 upx-4.1.0-amd64_linux/upx && ./upx -9 /go/src/github.com/traefik/mesh/dist/traefik-mesh

## IMAGE
FROM {{ .RuntimeImage }}

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chown=1000:1000 /go/src/github.com/traefik/mesh/dist/traefik-mesh /app/
USER app

ENTRYPOINT ["/app/traefik-mesh"]
