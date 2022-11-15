package testdata

import _ "embed"

//go:embed bindings/env_with_user_provided_service.json
var EnvWithUserProvidedIAS string

//go:embed bindings/env_with_ias_auth_with_client_secret.json
var EnvWithIASAuthWithClientSecret string

//go:embed bindings/env_with_ias_auth_x509.json
var EnvWithIASAuthX509 string

//go:embed bindings/env_with_ias_auth_x509_expired.json
var EnvWithIASAuthX509Expired string

//go:embed bindings/env_with_megaclite.json
var EnvWithMegaclite string

//go:embed bindings/env_with_megaclite_and_ias.json
var EnvWithMegacliteAndIAS string
