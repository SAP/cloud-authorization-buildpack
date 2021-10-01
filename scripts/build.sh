#!/usr/bin/env bash
set -euo pipefail
readonly SCRIPT_DIR=$(realpath "$( dirname "${BASH_SOURCE[0]}" )")

pushd "$SCRIPT_DIR/.."
#TODO: use Dockerfile for reproducible build
echo "Building sources.."
GOOS=linux go build -o bin/supply ./cmd/supply
echo "Packaging buildpack.."
buildpack-packager build --stack cflinuxfs3 --cached

popd
