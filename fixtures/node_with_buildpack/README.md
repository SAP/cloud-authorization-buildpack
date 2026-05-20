<!--
SPDX-FileCopyrightText: 2021 2020 2020 SAP SE or an SAP affiliate company and Cloud Security Client Go contributors

SPDX-License-Identifier: Apache-2.0
-->

# Simple NodeJS example using cloud-authorization-buildpack

## Description
This app has no real functionality. It just illustrates the authorzation sidecar usage. When pushed to CloudFoundry it does the following:
- Prints deprecation warning
- Uploads the DCL files to the AMS Server, where it gets compiled to a bundle and uploaded to an object store bucket
- Defines an NodeJS main process that idles just to keep the app alive

## Deployment
Navigate to the directory fixtures/node_with_buildpack

```sh
cf create-service identity application ias-node-buildpack -c identity.json
cf push
```