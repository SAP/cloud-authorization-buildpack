# ⚠️ Deprecation warning ⚠️

This repository is deprecated. Please migrate now.

- **Policy Upload**: For the base policy upload please use [the dedicated policies deployer](https://sap.github.io/cloud-identity-developer-guide/Authorization/DeployDCL.html) going foward
- **Policy decision engine**: As a policy decision engine please upgrade to [our latest official client libraries](https://sap.github.io/cloud-identity-developer-guide/Authorization/GettingStarted.html#dependency-setup) which have in-memory evaluations and no longer need the sidecar provided by this buildpack
- **Support**: To get support please use [our official support channels](https://sap.github.io/cloud-identity-developer-guide/Support.html)

# Buildpack User Documentation

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/cloud-authorization-buildpack)](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack)

A supply buildpack which uploads an application's base DCL policies to the
Authorization Management Service of a bound Identity Service instance. It does
not run stand-alone; combine it with a regular application buildpack (Java,
Node.js, …) as the *first* buildpack in the chain.

## Usage

Reference the latest released buildpack from your `manifest.yml` or the `-b` flag:

```
https://github.com/SAP/cloud-authorization-buildpack/releases/latest/download/opa_buildpack.zip
```

Avoid referencing a branch of this repo directly; doing so:

- adds a start-up dependency to `buildpacks.cloudfoundry.org`,
- significantly increases staging time, and
- may pull in unreleased breaking changes.

> ❗ Add this buildpack as the **first** buildpack (see the [fixture manifest.yml](https://github.com/SAP/cloud-authorization-buildpack/blob/main/fixtures/node_with_opa/manifest.yml)) — it is a supply buildpack and only contributes to the staging process. See also the [CF docs about multi-buildpack usage](https://docs.cloudfoundry.org/buildpacks/understand-buildpacks.html#:~:text=buildpack%20in%20the%20order%20is%20the%20final%20buildpack).

### Service binding

The buildpack expects exactly one bound Identity service instance with the
Authorization Management Service activated. It is found by parsing
`VCAP_SERVICES` for an entry with service type `identity`, or for any
user-provided service whose name or tag is `identity`.

The binding must contain a `certificate`, a `key`, the identity tenant `url`
and an `authorization_instance_id`.

To create such an identity instance, provide the following provisioning
parameters:

```json
{
  "authorization": {
    "enabled": true
  }
}
```

When binding the service instance to your application (or creating service
keys) the following parameters must be provided to obtain X.509 credentials.
The buildpack uses these to authenticate the policy upload to the AMS
service:

```json
{
  "credential_type": "X509_GENERATED"
}
```

#### DeployWithConfidence (DwC) support

DwC support is preserved. Where no Identity service is bound directly to the
app, the buildpack falls back to a user-provided service named `megaclite`
(containing its `url`) and uses the CF instance certificate for authentication.

### Base Policy Upload

By default the buildpack does **not** upload anything. To enable upload set the
environment variable `AMS_DCL_ROOT` to the path containing your `schema.dcl`
and DCL packages (relative to the application root). For example, in Spring
that is typically `/BOOT-INF/classes/`, in Java often `/WEB-INF/classes/`. The
buildpack uploads all `.dcl` files in all subfolders during staging.

If `AMS_DCL_ROOT` is unset, the buildpack only prints the deprecation warning
and otherwise does nothing.

## Development

Prerequisites:

* Go
* [buildpack-packer](https://github.com/cloudfoundry/libbuildpack/tree/master/packager#installing-the-packager)
* Make
* Docker

Run `make test` to run unit tests. Run `make build` to package the buildpack as a `.zip` file.

### Release process

Use GitHub to create a release:

1. Bump the [VERSION](/VERSION) file.
2. Run `make build` to create the packed buildpack.
3. Upload the resulting `opa_buildpack.zip` as a release asset.

## Reporting issues

Open an issue on this project. Note that this buildpack is in maintenance-only
mode; please direct authorization questions to the [official support channels](https://sap.github.io/cloud-identity-developer-guide/Support.html).

## Contributing

### Contributing with AI-generated code

As artificial intelligence evolves, AI-generated code is becoming valuable for many software projects, including open-source initiatives. While we recognize the potential benefits of incorporating AI-generated content into our open-source projects there a certain requirements that need to be reflected and adhered to when making contributions.

Please see our [guideline for AI-generated code contributions to SAP Open Source Software Projects](https://github.com/SAP/.github/blob/main/CONTRIBUTING_USING_GENAI.md) for these requirements.

## Licensing

Copyright 2020-2022 SAP SE or an SAP affiliate company and cloud-authorization-buildpack contributors. Please see
our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and
their licensing/copyright information is
available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/cloud-authorization-buildpack).
