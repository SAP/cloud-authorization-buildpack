package services

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/go-playground/validator/v10"
)

func fromMegaclite() (*AMSCredentials, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]MegacliteService
	err := json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
	}
	if ups, ok := svcs["user-provided"]; ok {
		for _, up := range ups {
			if up.Name == "megaclite" {
				megacliteURL := up.Credentials.URL
				if megacliteURL == "" {
					return nil, fmt.Errorf("invalid megaclite URL: %q", megacliteURL)
				}
				return &AMSCredentials{
					BundleURL:  megacliteURL + "/ams/bundle/",
					URL:        megacliteURL + "/ams/proxy/",
					InstanceID: MegacliteID,
				}, nil
			}
		}
	}
	return nil, nil
}

func fromIdentity(log Logger) (*AMSCredentials, error) {
	identityCreds, err := loadIdentityCreds(log)
	if identityCreds == nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not load identity credentials: %w", err)
	}
	if identityCreds.AuthzInstanceID == "" {
		return nil, nil
	}
	if identityCreds.Certificate == "" || identityCreds.Key == "" { // TODO: Remove the check for KEY once X509_PROVIDED bindings are supported
		return nil, fmt.Errorf(`invalid bindings credentials for identity service with AMS enabled: service bindings must be created with {"credential-type": "X509_GENERATED"} (more information in the identity broker documentation)`)
	}
	validate := validator.New()
	err = validate.Struct(identityCreds)
	if err != nil {
		return nil, fmt.Errorf("invalid binding credentials for identity service with AMS enabled: %w", err)
	}

	amsURL, err := url.Parse(identityCreds.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant URL in identity service %q", identityCreds.URL)
	}
	bundleURL := *amsURL // can safely be copied
	bundleURL.Path = "/bundle-gateway"
	amsURL.Path = "/authorization"
	return &AMSCredentials{
		BundleURL:  bundleURL.String(),
		URL:        amsURL.String(),
		InstanceID: identityCreds.AuthzInstanceID,
	}, nil
}

func loadIdentityCreds(log Logger) (*UnifiedIdentityCredentials, error) {
	iasService, err := LoadService(log, "identity")
	if iasService == nil {
		return nil, err
	}
	var iasCreds UnifiedIdentityCredentials
	err = json.Unmarshal(iasService.Credentials, &iasCreds)
	return &iasCreds, err
}
