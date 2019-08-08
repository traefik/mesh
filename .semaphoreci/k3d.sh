#!/usr/bin/env bash

set -e

curl -s https://raw.githubusercontent.com/rancher/k3d/v1.3.1/install.sh | bash

which k3d

k3d --version
