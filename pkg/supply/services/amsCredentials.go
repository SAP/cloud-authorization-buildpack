package services

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudfoundry/libbuildpack"
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

func fromIdentity(log *libbuildpack.Logger) (*AMSCredentials, error) {
	identityCreds, err := loadIdentityCreds(log)
	if err != nil {
		return nil, fmt.Errorf("could not load identity credentials: %w", err)
	}
	if identityCreds.AuthzURL == "" {
		return nil, nil
	}
	validate := validator.New()
	err = validate.Struct(identityCreds)
	return &AMSCredentials{
		BundleURL:   identityCreds.AuthzBundleURL,
		ObjectStore: identityCreds.AuthzObjectStore,
		URL:         identityCreds.AuthzURL,
		InstanceID:  identityCreds.AuthzInstanceID,
	}, err
}

func loadIdentityCreds(log *libbuildpack.Logger) (UnifiedIdentityCredentials, error) {
	iasService, err := LoadService(log, "identity")
	if err != nil {
		return UnifiedIdentityCredentials{}, err
	}
	var iasCreds UnifiedIdentityCredentials
	err = json.Unmarshal(iasService.Credentials, &iasCreds)
	return iasCreds, err
}

func fromAuthz(log *libbuildpack.Logger, serviceName string) (*AMSCredentials, error) {
	amsService, err := LoadService(log, serviceName)
	if err != nil {
		return nil, err
	}
	var amsCreds AMSCredentials
	if err := json.Unmarshal(amsService.Credentials, &amsCreds); err != nil {
		return nil, err
	}
	validate := validator.New()
	if err := validate.Struct(amsCreds); err != nil {
		return nil, err
	}
	if len(amsCreds.InstanceID) == 0 {
		if len(amsService.InstanceID) == 0 {
			return nil, fmt.Errorf("authorization credentials bound via user-provided-service, however parameter instance_id is missing. Please update the binding")
		}
		amsCreds.InstanceID = amsService.InstanceID // legacy mode, until all consumers have bindings with integrated instance_id
	}
	return &amsCreds, err
}
