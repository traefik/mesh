FROM alpine:3.18

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
    ruby-nokogiri \
    ruby-dev \
    build-base

RUN gem install --no-document html-proofer -v 5.0.8 -- --use-system-libraries

# After Ruby, some NodeJS YAY!
RUN apk --no-cache --no-progress add \
    git \
    nodejs \
    npm

# unsafe-perm true to handle 'not get uid/gid'
RUN npm install --global --unsafe-perm=true \
    markdownlint@0.31.1 \
    markdownlint-cli@0.37.0

# Finally the shell tools we need for later
# tini helps to terminate properly all the parallelized tasks when sending CTRL-C
RUN apk --no-cache --no-progress add \
    ca-certificates \
    curl \
    tini

COPY ./scripts/verify.sh ./scripts/lint.sh /

WORKDIR /app
VOLUME ["/tmp","/app"]

ENTRYPOINT ["/sbin/tini","-g","sh"]
