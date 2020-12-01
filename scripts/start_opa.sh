#!/bin/bash

env

cat $opa_config
$opa_binary run -s -c $opa_config --log-level=$log_level --addr=[]:$OPA_PORT