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

>❗️ Add this buildpack as the first buildpack as shown in the [fixture manifest.yml](https://github.com/SAP/cloud-authorization-buildpack/blob/main/fixtures/node_with_opa/manifest.yml) as it only supplies dependencies. See also the [CF docs about multi-buildpack usage](https://docs.cloudfoundry.org/buildpacks/understand-buildpacks.html#:~:text=buildpack%20in%20the%20order%20is%20the%20final%20buildpack).

### Services

#### Identity Service

This buildpack expects to find a bound identity service with Authorization Management Service activated. To find the
service it parses the service bindings in the VCAP_SERVICES with service type `identity` or any user-provided services
with the name or tag `identity`. Only one matching service binding is allowed. The service binding is expected to
contain a "certificate", a "key", the identity tenant `url` and the `authorization_instance_id`.

To create such an identity instance you need to provide the following provisioning parameters:

```json
{
  "authorization": {
      "enabled": "true"
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

### Updating SAP-OPA
1. download latest linux-amd64 binary from repository: https://common.repositories.cloud.sap/ui/native/deploy.releases/com/sap/golang/github/wdf/sap/corp/cpsecurity/cas-opa-sap/ in an empty folder
2. go to the folder where it was downloaded and run `tar -xf {}.tar.gz` to unzip archive
3. go into the folder `cd linux-amd64`
4. zip the binary `tar -czvf opa.tar.gz opa` 
5. generate the SHA256 for the archive 
   1. linux `cat opa.tar.gz | sha256sum` 
   2. macOS `shasum -a 256 opa.tar.gz` 
6. update the SHA256 checksum in [manifest.yml](/manifest.yml) dependencies->opa->sha256 
7. and place `opa.tar.gz` in resources folder
8. update the version [manifest.yml](/manifest.yml) dependencies->opa->version & default_versions->opa->version
9. update the OPA version in [go.mod](go.mod) and run `go mod tidy`

### Release Process
Use github to create a release
1. upgrade [VERSION](/VERSION) file 
2. execute make build to create a packed buildpack
3. upload the packed buildpack (opa_buildpack.zip) as asset to the release

## Reporting Issues

Open an issue on this project

## Disclaimer

This buildpack is experimental and not yet intended for production use.

## Licensing

Copyright 2020-2022 SAP SE or an SAP affiliate company and cloud-authorization-buildpack contributors. Please see
our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and
their licensing/copyright information is
available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack).
