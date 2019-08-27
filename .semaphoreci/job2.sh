#!/usr/bin/env bash
set -e

if [ -n "$SHOULD_TEST" ]; then ci_retry make local-test-integration; fi
