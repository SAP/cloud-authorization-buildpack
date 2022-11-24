package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	AmsServerPath        = "authorization"
	AmsBundleGatewayPath = "bundle-gateway"

	MegacliteID                   = "dwc-megaclite-ams-instance-id"
	MegacliteAmsServerPath        = "/ams/proxy/"
	MegacliteAmsBundleGatewayPath = "/ams/bundle/"
)

var (
	ErrServiceNotFound = errors.New("service not found")
)

type Service struct {
	Name        string          `json:"name"`
	Tags        []string        `json:"tags"`
	Credentials json.RawMessage `json:"credentials"`
	InstanceID  string          `json:"instance_guid"`
}

type IASCredentials struct {
	Certificate          string    `json:"certificate" validate:"required"`
	CertificateExpiresAt time.Time `json:"certificate_expires_at"`
	ClientID             string    `json:"clientid"`
	Domain               string    `json:"domain"`
	Domains              []string  `json:"domains"`
	Key                  string    `json:"key" validate:"required"`
	OsbURL               string    `json:"osb_url"`
	ProoftokenURL        string    `json:"prooftoken_url"`
	URL                  string    `json:"url"`
	ZoneUUID             string    `json:"zone_uuid"`

	AmsInstanceID string `json:"authorization_instance_id"  validate:"required"`
	AmsClientID   string `json:"authorization_client_id"`

	// derived values
	AmsServerURL        string
	AmsBundleGatewayURL string
}

type MegacliteCredentials struct {
	URL string `json:"url"`
}

func LoadService(log Logger, serviceName string) (*Service, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]Service
	err := json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
	}

	filteredServices := make([]Service, 0, 1)
	if ups, ok := svcs["user-provided"]; ok {
		for i := range ups {
			if ups[i].Name == serviceName {
				log.Info("Detected user-provided '%s' service '%s' via its name", serviceName, ups[i].Name)
				ups[i].InstanceID = "" // delete since it's the instance id of the user-provided-service, not the actual instance
				filteredServices = append(filteredServices, ups[i])
				continue
			}
			for _, t := range ups[i].Tags {
				if t == serviceName {
					log.Info("Detected user-provided '%s' service '%s' via its tag '%s'", serviceName, ups[i].Name, t)
					ups[i].InstanceID = "" // delete since it's the instance id of the user-provided-service, not the actual instance
					filteredServices = append(filteredServices, ups[i])
				}
			}
		}
	}
	filteredServices = append(filteredServices, svcs[serviceName]...)
	if len(filteredServices) > 1 {
		return nil, fmt.Errorf("expect only one service (type %s or user-provided) but got %d", serviceName, len(filteredServices))
	} else if len(filteredServices) < 1 {
		return nil, ErrServiceNotFound
	}
	return &filteredServices[0], nil
}

func LoadServiceCredentials(log Logger) (*IASCredentials, error) {
	creds, err := fromIdentity(log)
	if !errors.Is(err, ErrServiceNotFound) { // if service is not found try to find megaclite
		return creds, err // return creds or err (one of them is nil)
	}

	creds, err = fromMegaclite(log)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			return nil, errors.New("cannot find authorization-enabled identity service")
		}
		return nil, fmt.Errorf("error trying to fallback to megaclite proxy to upload AMS DCLs and download AMS bundles: %w", err)
	}

	log.Info("using megaclite proxy to upload AMS DCLs and download AMS bundles")
	return creds, nil
}
