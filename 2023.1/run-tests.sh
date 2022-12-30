#!/usr/bin/env bash

# basic sanity tests that check that an image can be built from Dockerfile and run, all linter options printed

for dir in */; do
    echo "Testing $dir"
    (cd "$dir" && docker buildx build -t qodana:dev . && docker run --rm -it qodana:dev -h )
done
