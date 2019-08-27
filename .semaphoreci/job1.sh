#!/usr/bin/env bash
set -e

make
if [ "$BRANCH_NAME" = "master" ]; then make push-docker ; fi
