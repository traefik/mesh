#!/usr/bin/env bash
set -e

if [ -n "$SHOULD_TEST" ]; then curl -s https://raw.githubusercontent.com/rancher/k3d/v1.3.1/install.sh | bash; fi
if [ -n "$SHOULD_TEST" ]; then which k3d; fi
if [ -n "$SHOULD_TEST" ]; then k3d --version; fi
if [ -n "$SHOULD_TEST" ]; then ci_retry make test-integration; fi
