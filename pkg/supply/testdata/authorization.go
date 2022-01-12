package testdata

import _ "embed"

//go:embed bindings/env_with_authorization.json
var EnvWithAuthorization string

//go:embed bindings/env_with_authorization_dev.json
var EnvWithAuthorizationDev string

//go:embed bindings/authorization-dev_bundle-url.json
var EnvWithUPSBundleURL string

//go:embed bindings/env_with_user_provided_service.json
var EnvWithUserProvidedAuthorization string

//go:embed bindings/env_with_ias_auth_with_client_secret.json
var EnvWithIASAuthWithClientSecret string

//go:embed bindings/env_with_ias_auth_x509.json
var EnvWithIASAuthX509 string

//go:embed bindings/env_with_ias_auth_x509_expired.json
var EnvWithIASAuthX509Expired string

//go:embed bindings/env_with_megaclite.json
var EnvWithMegaclite string
