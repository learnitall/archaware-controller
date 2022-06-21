#!/bin/bash
#
# Build manifests and indices for docker and quay
# Requires that 'archaware-testimg' is created
# in both quay.io and hub.docker.com
#
# Run from testimg directory, not project root
#
set -eo pipefail

if [ -z "${3}" ]
then
    echo "usage: $0 docker_namespace quay_namespace quay_auth_file"
    exit 1
fi

DOCKER_NS=$1
QUAY_NS=$2
QUAY_AUTH_FILE=$3

function update_img {
    set -x
    $1 build . -t $2/$3/archaware-testimg:$4 -f Containerfile --platform linux/$4
    export REGISTRY_AUTH_FILE=$5
    $1 push $2/$3/archaware-testimg:$4
    set +x
}

for platform in amd64 arm
do
    update_img "docker" "docker.io" $DOCKER_NS $platform $DOCKER_USER
    update_img "podman" "quay.io" $QUAY_NS $platform $QUAY_AUTH_FILE
done

set -x

tag=index

img=docker.io/$DOCKER_NS/archaware-testimg
docker manifest create --amend $img:$tag \
    $img:amd64 \
    $img:arm
docker manifest push $img:$tag
docker manifest inspect $img

export REGISTRY_AUTH_FILE=$QUAY_AUTH_FILE
img=quay.io/$QUAY_NS/archaware-testimg
podman manifest create --all $img:$tag \
    $img:amd64 \
    $img:arm
podman push $img:$tag
docker manifest inspect $img
set +x