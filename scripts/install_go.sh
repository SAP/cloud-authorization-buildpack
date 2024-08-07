#!/bin/bash

# SPDX-FileCopyrightText: Dr Nic Williams 2017-Present
# SPDX-License-Identifier: LicenseRef-mit-drnic

set -e
set -u
set -o pipefail

function main() {
  if [[ "${CF_STACK:-}" != "cflinuxfs3" && "${CF_STACK:-}" != "cflinuxfs4" ]]; then
    echo "       **ERROR** Unsupported stack"
    echo "                 See https://docs.cloudfoundry.org/devguide/deploy-apps/stacks.html for more info"
    exit 1
  fi

  local version expected_sha dir
  version="1.22.5"
  expected_sha="4dbc8e16ac679e5ba50b26d5f4fbd4fddd82d7a3612a3082ed33fb4472d2b3c3"
  dir="/tmp/go${version}"

  mkdir -p "${dir}"

  if [[ ! -f "${dir}/go/bin/go" ]]; then
    local url
    # TODO: use exact stack based dep, after go buildpack has cflinuxfs4 support
    #url="https://buildpacks.cloudfoundry.org/dependencies/go/go_${version}_linux_x64_${CF_STACK}_${expected_sha:0:8}.tgz"
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
