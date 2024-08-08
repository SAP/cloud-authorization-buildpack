#!/bin/bash

if ! ls /etc/secrets/sapbtp/identity; then
    >&2 echo "Error: No identity service found at /etc/secrets/sapbtp/identity"
    exit 1
fi
files=(/etc/secrets/sapbtp/identity/*)
if [ ${#files[@]} -gt 1 ]; then
  >&2 echo "Error: More than one identity service found at /etc/secrets/sapbtp/identity"
  exit 1
fi

bundle_url=$(cat "${files[0]}/authorization_bundle_url")
instance_id=$(cat "${files[0]}/authorization_instance_id")
ias_cert_path="${files[0]}/certificate"
ias_key_path="${files[0]}/key"

jq -n --arg bundleUrl "$bundle_url" --arg iasCertPath "$ias_cert_path" --arg iasKeyPath "$ias_key_path" --arg instanceResource "$instance_id.tar.gz" --arg instanceID "$instance_id" -f config-template.json >config.yml

>&2 echo "INFO: " "$(cat config.yml)"

opa run -s -c config.yml --set status.plugin=dcl --addr=[]:8181