#!/bin/bash
#
# This script will launch the dockerd engine in background
# and try to load the docker-cache if tar file founds

set -eu

DOCKER_LOGS="/var/log/docker.log"

wait_for_docker_startup_or_exit() {
  # Wait for Docker startup
  counter=0
  max_attempts=30
  until [ "${counter}" -ge "${max_attempts}" ]
  do
    set +e
    curl -s --fail http://localhost:2375/_ping >/dev/null && break
    set -e
    counter=$((counter+1))
    sleep 1
  done
  [ "${counter}" -lt "${max_attempts}" ] || cat "${DOCKER_LOGS}"
}

####
# Launch the DinD startup in background
# only if START_DOCKER is true (default)
###

if [ "${START_DOCKER}" = "true" ]
then

  echo "== Starting Docker in background. Logs can be found in ${DOCKER_LOGS}."
  # Launch Docker Engine (dind= Docker in Docker) in background, in debug, outputing to DOCKER_LOGS
  bash -x /usr/local/bin/dockerd-entrypoint.sh >"${DOCKER_LOGS}" 2>&1 &

  # Wait for docker engine startup
  wait_for_docker_startup_or_exit

else
  echo "== START_DOCKER has the value: ${START_DOCKER}. Not starting Docker."
fi

echo "== Docker started successfully"

"$@"
