package services

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-playground/validator/v10"
)

func fromMegaclite(log Logger) (*IASCredentials, error) {
	megacliteService, err := LoadService(log, "megaclite")
	if err != nil {
		return nil, err
	}

	var megacliteCreds MegacliteCredentials
	err = json.Unmarshal(megacliteService.Credentials, &megacliteCreds)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling identity credentials: %w", err)
	}

	if megacliteCreds.URL == "" {
		return nil, fmt.Errorf("invalid megaclite URL: %q", megacliteCreds.URL)
	}

	result := IASCredentials{AmsInstanceID: MegacliteID}
	result.AmsServerURL, err = url.JoinPath(megacliteCreds.URL, MegacliteAmsServerPath)
	if err != nil {
		return nil, fmt.Errorf("error building ams server url: %w", err)
	}
	result.AmsBundleGatewayURL, err = url.JoinPath(megacliteCreds.URL, MegacliteAmsBundleGatewayPath)
	if err != nil {
		return nil, fmt.Errorf("error building bundle gateway url: %w", err)
	}

	return &result, nil
}

func fromIdentity(log Logger) (*IASCredentials, error) {
	iasService, err := LoadService(log, "identity")
	if err != nil {
		return nil, err
	}
	var creds IASCredentials
	err = json.Unmarshal(iasService.Credentials, &creds)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling identity credentials: %w", err)
	}

	// explicit checks to improve error messages for consumer
	if creds.AmsInstanceID == "" {
		return nil, fmt.Errorf("identity service credentials found without activated authorization management service")
	}

	if creds.Certificate == "" || creds.Key == "" { // TODO: Remove the check for KEY once X509_PROVIDED bindings are supported
		return nil, fmt.Errorf(`invalid bindings credentials for identity service with AMS enabled: service bindings must be created with {"credential-type": "X509_GENERATED"} (more information in the identity broker documentation)`)
	}

	if creds.CertificateExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("identity certificate has expired: %s. Please re-create your identity binding", creds.CertificateExpiresAt.String())
	}

	validate := validator.New()
	err = validate.Struct(creds)
	if err != nil {
		return nil, fmt.Errorf("invalid binding credentials for identity service with AMS enabled: %w", err)
	}

	creds.AmsServerURL = creds.URL

	return &creds, nil
}
