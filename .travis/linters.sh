#!/usr/bin/env bash
set -exu

go get -v -u github.com/golangci/golangci-lint/cmd/golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint
cd "${TRAVIS_BUILD_DIR}"
golangci-lint run --enable deadcode --enable varcheck --enable staticcheck

# check license headers
# this needs to be run from the top level directory, because it uses
# `git ls-files` under the hood.
go get -v -u github.com/u-root/u-root/tools/checklicenses
go install github.com/u-root/u-root/tools/checklicenses
cd "${TRAVIS_BUILD_DIR}"
echo "[*] Running checklicenses"
go run github.com/u-root/u-root/tools/checklicenses -c .travis/config.json
