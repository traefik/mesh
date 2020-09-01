FROM golang:1.15-alpine AS builder

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

WORKDIR /go/src/github.com/containous/maesh

# Download goreleaser binary to bin folder in $GOPATH
RUN curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sh

ENV GO111MODULE on
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GOARCH={{ .GoARCH }} GOARM={{ .GoARM }} make local-build

## IMAGE
FROM {{ .RuntimeImage }}

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/containous/maesh/dist/maesh /app/

ENTRYPOINT ["/app/maesh"]
