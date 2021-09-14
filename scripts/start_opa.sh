#!/bin/bash

# SPDX-FileCopyrightText: 2021 2020 2020 SAP SE or an SAP affiliate company and Cloud Security Client Go contributors
#
# SPDX-License-Identifier: Apache-2.0

env

cat $opa_config
$opa_binary run -s -c $opa_config --log-level=$log_level --addr=[]:$OPA_PORT --set plugins.dcl=true
