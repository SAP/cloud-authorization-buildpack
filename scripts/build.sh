#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2020 2020 SAP SE or an SAP affiliate company and Cloud Security Client Go contributors
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail
readonly SCRIPT_DIR=$(realpath "$( dirname "${BASH_SOURCE[0]}" )")

echo "Removing bash supply script (won't be used in packaged buildpack)"
rm "$SCRIPT_DIR/../bin/supply" || echo "$SCRIPT_DIR/../bin/supply not found"
echo "Building sources.."
ENABLE_CGO=0 GOARCH=amd64 GOOS=linux go build -o bin/supply ./cmd/supply
echo "Done"
