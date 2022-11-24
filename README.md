# Buildpack User Documentation

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/cloud-authorization-buildpack)](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack)

This is a supply/sidecar buildpack which can't be used stand-alone. It has two major purposes. It defines a sidecar
process which handles the authorization decisions. This sidecar is queried by the security client libraries. And it
provides an upload mechanism for the applications base policy definitions to the Authorization Management Service.

## Usage

Consume the latest released version of this buildpack with the following link in your manifest.yml or via the `-b` flag:

https://github.com/SAP/cloud-authorization-buildpack/releases/latest/download/opa_buildpack.zip  
We discourage referencing a branch of this repo directly because:

- adds a start-up dependency to buildpacks.cloudfoundry.org, which should be avoided
- staging time will be increased significantly
- may contain potentially breaking changes

### Services

This buildpack expects to find a bound identity service with Authorization Management Service activated. To find the
service it parses the service bindings in the VCAP_SERVICES with service type `identity` or any user-provided services
with the name or tag `identity`. Only one matching service binding is allowed. The service binding is expected to
contain a "certificate", a "key", the identity tenant `url` and the `authorization_instance_id`.

To create such an identity instance you need to provide the following provisioning parameters:

```json
{
  "authorization": {
    "product_label": "<some text for the UI>"
  }
}
```

When binding the service instance to your application or when creating service keys the following parameters must be
provided in order to create certificate based credentials. These are used by the buildpack to upload the policies to
your service instance and to download the authorization bundle during runtime.

```json
{
  "credential_type": "X509_GENERATED"
}
```

#### Support for DeployWithConfidence (DwC)

There is also DwC support, where no services are bound directly to the app. All communication will be proxied by the
megaclite component of DwC. Therefor a user-provided service with name "megaclite" is expected, containing its "url".

### Base Policy Upload

By default this buildpack doesn't upload any policies. To upload the base policies you need to provide the environment
variable AMS_DCL_ROOT with the value of the path that contains the schema.dcl and the DCL packages. (For example in
Spring
`/BOOT-INF/classes/` or `/WEB-INF/classes/` in Java; For other main buildpacks just the absolute folder relative to the
project root). The buildpack will then upload all DCL files in all subfolders at the app staging.

## Development

Prerequisites:

* Go
* [buildpack-packer](https://github.com/cloudfoundry/libbuildpack/tree/master/packager#installing-the-packager)
* Make
* Docker

Run `make test` to run unit tests. Run `make build` to package the buildpack as a .zip file.

## Reporting Issues

Open an issue on this project

## Disclaimer

This buildpack is experimental and not yet intended for production use.

## Licensing

Copyright 2020-2022 SAP SE or an SAP affiliate company and cloud-authorization-buildpack contributors. Please see
our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and
their licensing/copyright information is
available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack).
