<!--
SPDX-FileCopyrightText: 2021 2020 2020 SAP SE or an SAP affiliate company and Cloud Security Client Go contributors

SPDX-License-Identifier: Apache-2.0
-->

# Simple NodeJS example using cloud-authorization buildpack

## Description
This app has no real functionality. It just illustrates the authorzation sidecar usage. When pushed to CloudFoundry it does the following:
- Uploads the DCL files to the AMS Server, where it gets compiled to a bundle and uploaded to an object store bucket
- Configures a sidecare process, that permanently syncs the authorization data from the object store bucket and hosts a server that can be queried about authorization
- Sets ADC_URL in the environment of the main process.The authorization queries should be sent there. The SAP security client libraries will automatically read this variable
- Defines an NodeJS main process that idles just to keep the app alive

To actually see something running you could connect to the app container using 
```sh
cf ssh node-opa
```
and query the sidecar process using curl
```sh
curl 127.0.0.1:8999
```


## Deployment
Navigate to the directory fixtures/node_with_opa

```sh
cf create-service identity application ias-node-opa -c identity.json
cf push
```
