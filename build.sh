#!/bin/bash

BUILD_DIR=bin
NAME=aws-env

mkdir $BUILD_DIR

platforms=("windows/amd64" "windows/386" "darwin/amd64" "linux/amd64" "linux/386" "linux/arm" "linux/arm64")

for platform in "${platforms[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    echo "Building $NAME for $GOOS/$GOARCH"
    GOOS=$GOOS GOARCH=$GOARCH go build -v -o $BUILD_DIR/$NAME-$GOOS-$GOARCH
done

