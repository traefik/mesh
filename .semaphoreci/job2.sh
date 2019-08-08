#!/usr/bin/env bash
set -e

if [ -n "$SHOULD_TEST" ]
then
    curl -s https://raw.githubusercontent.com/rancher/k3d/v1.3.1/install.sh | bash
    which k3d
    k3d --version
    ci_retry make local-test-integration
fi
