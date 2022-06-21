#!/bin/bash
set -xeo pipefail

docker buildx build . \
    -t $1/archaware-controller:$2 \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    -f Containerfile \
    --platform linux/amd64,linux/arm \
    --push
