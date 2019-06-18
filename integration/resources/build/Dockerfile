FROM golang:1.12-alpine as golang-image

# Minimalistic image
FROM docker:18.09-dind
ENV START_DOCKER="true"

RUN apk add --no-cache bash ca-certificates curl sudo tini wget jq openssl py-pip python-dev libffi-dev openssl-dev gcc libc-dev make git \
  && ln -s /usr/local/bin/docker* /usr/bin/ \
  && echo "= Installing docker-compose..." \
  && pip install --no-cache-dir \
    docker-compose==1.24.0

COPY ./entrypoint.sh /usr/bin/entrypoint.sh

VOLUME ["/var/log"]

WORKDIR "/i3o"

RUN mkdir /usr/local/go
COPY --from=golang-image /usr/local/go /usr/local/go

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

# Use tini to catch any stop signal
ENTRYPOINT ["/sbin/tini","-g","--", "/bin/bash","/usr/bin/entrypoint.sh"]
# Cleanup any CMD inherited from parent image
CMD []
