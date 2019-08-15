#!/bin/bash
NAME=aws-env
for GOOS in darwin linux windows; do
    for GOARCH in 386 amd64; do
        GOOS=$GOOS GOARCH=$GOARCH go build -v -o $BUILD_DIR/$NAME-$GOOS-$GOARCH
    done
done
