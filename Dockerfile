# Let's build i3o for linux-amd64
FROM golang:1.12-alpine AS base-image

# Package dependencies
RUN apk --no-cache --no-progress add \
    bash \
    gcc \
    git \
    make \
    musl-dev

ENV PROJECT_WORKING_DIR=/go/src/github.com/containous/i3o

WORKDIR "${PROJECT_WORKING_DIR}"
COPY go.mod go.sum "${PROJECT_WORKING_DIR}"/
RUN GO111MODULE=on go mod download
COPY . "${PROJECT_WORKING_DIR}/"

FROM base-image as maker

ARG MAKE_TARGET=local-build

RUN make ${MAKE_TARGET}

## IMAGE
FROM alpine:3.9

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

COPY --from=maker /go/src/github.com/containous/i3o/dist/i3o /app/

ENTRYPOINT ["/app/i3o"]
