#!/usr/bin/env bash
set -e

echo "Building for Linux"
env GOOS=linux go build ./...

echo "Building for FreeBSD"
env GOOS=freebsd go build ./...

echo "Building for Mac"
env GOOS=darwin go build ./...