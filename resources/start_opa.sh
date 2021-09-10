#!/bin/bash

if [ "${log_level,,}" = "debug" ]; then
  env
  cat $opa_config
fi;

set -euo pipefail

$opa_binary run -s -c $opa_config --log-level=$log_level --addr=[]:$OPA_PORT
