FROM alpine:3.10

# The "build-dependencies" virtual package provides build tools for html-proofer installation.
# It compile ruby-nokogiri, because alpine native version is always out of date
# This virtual package is cleaned at the end.
RUN apk --no-cache --no-progress add \
    libcurl \
    ruby \
    ruby-bigdecimal \
    ruby-etc \
    ruby-ffi \
    ruby-json \
    ruby-nokogiri
RUN NOKOGIRI_USE_SYSTEM_LIBRARIES=true gem install --no-document html-proofer -v 3.12.0

# After Ruby, some NodeJS YAY!
RUN apk --no-cache --no-progress add \
    git \
    nodejs \
    npm \
  && npm install markdownlint@0.16.0 markdownlint-cli@0.18.0 --global

# Finally the shell tools we need for later
# tini helps to terminate properly all the parallelized tasks when sending CTRL-C
RUN apk --no-cache --no-progress add \
    ca-certificates \
    curl \
    tini

COPY ./scripts/verify.sh /verify.sh
COPY ./scripts/lint.sh /lint.sh

WORKDIR /app
VOLUME ["/tmp","/app"]

ENTRYPOINT ["/sbin/tini","-g","sh"]
