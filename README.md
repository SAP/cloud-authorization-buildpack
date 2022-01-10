# Buildpack User Documentation

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/cloud-authorization-buildpack)](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack)

This is a supply/sidecar buildpack which can't be used stand-alone. It has two major purposes. It defines a sidecar process which handles the authorization decisions. This sidecar is queried by the security client libraries. And it provides an upload mechanism for the applications base policy definitions to the Authorization Management Service.

## Usage
Consume the latest released version of this buildpack with the following link in your manifest or via the `-b` flag:

https://github.com/SAP/cloud-authorization-buildpack/releases/latest/download/opa_buildpack.zip  
We discourage referencing a branch of this repo directly because:
 - adds a start-up dependency to buildpacks.cloudfoundry.org, which we should avoid
 - staging time will be increased significantly
 - may contain potentially breaking changes

### Services
#### Authorization Service (Legacy)
This buildpack expects to find a bound identity service containing "cert" and "key" values in the credentials. This instance must have registered an authorization instance as consumed service instance. This authorization instance also needs to be bound to this app and will be identified as follows:   
By default this buildpack expect to find an "authorization" service binding in the VCAP_SERVICES.
It's also possible to bind a user-provided service instead, when it has same structure as the "authorization" binding and is tagged with "authorization". Another way to override this behavior is to provide the environment variable AMS_SERVICE to target another service than "authorization"(e.g. "authorization-dev")
#### Identity Service 
The buildpack expects to find a bound identity service containing "cert" and "key" values in the credentials, as well as authorization values (e.g. "authorization_url"). To create such an identity instance you need to provide the following provisioning parameters:
´´´
{
    "credential_type": "X509_GENERATED",
    "authorization": {
        "product_label":"<some text for the UI>"
    }
}
´´´
#### Support for DeployWithConfidence (DwC)
There is also DwC support, where no services are bound directly to the app. All communication will be proxied by the megaclite component of DwC. Therefor a user-provided service with name "megaclite" is expected, containing its "url".
### Base Policy Upload
By default this buildpack doesn't upload any policies. To upload the base policies, provide the environment variable AMS_DCL_ROOT with the value of the path that contains the schema.dcl and the DCL packages. (For example in Spring /META-INF/classes; For other main buildpacks just the absolute folder relative to the project root). The buildpack will then upload all DCL files in all subfolders at the app staging. This enviromnent variable will be probably be replaced with an AMS config file end of Q4 2021(https://jtrack.wdf.sap.corp/browse/SECAUTH-1534)

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
Copyright 2020-2021 SAP SE or an SAP affiliate company and cloud-authorization-buildpack contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack).
