#!/usr/bin/env sh

set -ex

echo "Running localtest!"

if [ -x "$(command -v docker)" ]; then
    echo "Running using docker"
    docker compose --profile "*" down -v
    docker compose --profile "*" up -d --build
elif [ -x "$(command -v podman)" ]; then
    echo "Running using podman"
    # TODO implement
fi
