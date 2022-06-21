#!/bin/bash
set -xeo pipefail

docker buildx build . \
    -t $DOCKER_NS/archaware-controller:latest \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    -f Containerfile \
    --platform linux/amd64,linux/arm \
    --push
