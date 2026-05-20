#!/bin/bash

# SPDX-FileCopyrightText: Dr Nic Williams 2017-Present
# SPDX-License-Identifier: LicenseRef-mit-drnic

set -e
set -u
set -o pipefail

function main() {
  if [[ "${CF_STACK:-}" != "cflinuxfs4" ]]; then
    echo "       **ERROR** Unsupported stack ('${CF_STACK:-}'). This buildpack only supports 'cflinuxfs4'."
    echo "                 See https://docs.cloudfoundry.org/devguide/deploy-apps/stacks.html for more info"
    exit 1
  fi

  local version expected_sha dir
  # Versions tracked from cloudfoundry/go-buildpack release manifest:
  # https://github.com/cloudfoundry/go-buildpack/releases
  version="1.26.2"
  expected_sha="95ecb6a3354688e1fc1ee1a9e5498657a26fc819e777a840160578cd4ca17452"
  dir="/tmp/go${version}"

  mkdir -p "${dir}"

  if [[ ! -f "${dir}/bin/go" ]]; then
    local url
    url="https://buildpacks.cloudfoundry.org/dependencies/go/go_${version}_linux_x64_cflinuxfs4_${expected_sha:0:8}.tgz"

    echo "-----> Download go ${version}"
    curl "${url}" \
      --silent \
      --location \
      --retry 15 \
      --retry-delay 2 \
      --output "/tmp/go.tgz"

    local sha
    sha="$(shasum -a 256 /tmp/go.tgz | cut -d ' ' -f 1)"

    if [[ "${sha}" != "${expected_sha}" ]]; then
      echo "       **ERROR** SHA256 mismatch: got ${sha}, expected ${expected_sha}"
      exit 1
    fi

    tar xzf "/tmp/go.tgz" -C "${dir}"
    rm "/tmp/go.tgz"
  fi

  if [[ ! -f "${dir}/bin/go" ]]; then
    echo "       **ERROR** Could not download go"
    exit 1
  fi

  GoInstallDir="${dir}"
  export GoInstallDir
}

main "${@:-}"