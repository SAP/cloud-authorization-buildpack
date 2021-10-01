# Buildpack User Documentation

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/cloud-authorization-buildpack)](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack)

## Usage
This is a supply/sidecar buildpack. Which can't be used stand-alone. It has two major purposes. It defines a sidecar process which handles the authorization decisions. This sidecar is queried by the security client libraries. And it provides an upload mechanism for the applications base policy definitions to the Authorization Management Service.

### Services
By default this buildpack expect to find an "authorization" service binding in the VCAP_SERVICES.
It's also possible to bind a user-provided service instead, when it has same structure as the "authorization" binding and is tagged with "authorization". Another way to override this behavior is to provide the environment variable AMS_SERVICE to target another service than "authorization"(e.g. "authorization-dev")
### Base Policy Upload
By default this buildpack doesn't upload any policies. To upload the base policies, provide the environment variable AMS_DCL_ROOT with the value of the path that contains the schema.dcl and the DCL packages. (For example in Spring /META-INF/classes; For other main buildpacks just the absolute folder relative to the project root). The buildpack will then upload all DCL files in all subfolders at the app staging.

## Development

Prerequisites:
* Go
* [buildpack-packer](https://github.com/cloudfoundry/libbuildpack/tree/master/packager#installing-the-packager)
* Make

Run `make test` to run unit tests. Run `make build` to package the buildpack as a .zip file.

## Reporting Issues
Open an issue on this project

## Disclaimer
This buildpack is experimental and not yet intended for production use.

## Licensing
Copyright 2020-2021 SAP SE or an SAP affiliate company and cloud-authorization-buildpack contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack).
