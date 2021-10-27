#SPDX-FileCopyrightText: 2020 The OPA Authors
#
#SPDX-License-Identifier: Apache-2.0
package system.health

# opa is live if it can process this rule
default live = true

# by default, opa is not ready
default ready = false

# opa is ready once all plugins have reported OK at least once
ready {
  input.plugins_ready
}