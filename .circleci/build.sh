#!/usr/bin/env bash
set -e

echo "Building for Linux"
env GOOS=linux go build ./...
