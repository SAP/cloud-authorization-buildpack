package testdata

import _ "embed"

//go:embed bindings/env_with_authorization.json
var EnvWithAuthorization string

//go:embed bindings/env_with_authorization_dev.json
var EnvWithAuthorizationDev string

//go:embed bindings/env_with_user_provided_service.json
var EnvWithUserProvidedAuthorization string

//go:embed bindings/env_with_ias_auth_with_client_secret.json
var EnvWithIASAuthWithClientSecret string

//go:embed bindings/env_with_ias_auth_x509.json
var EnvWithIASAuthX509 string
