#!/bin/bash
NAME=aws-env
for GOOS in darwin linux windows; do
    for GOARCH in 386 amd64; do
        echo "Building $NAME-$GOOS-$GOARCH"
        GOOS=$GOOS GOARCH=$GOARCH go build -o $BUILD_DIR/$NAME-$GOOS-$GOARCH
    done
done
