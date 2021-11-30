package supply

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/go-playground/validator/v10"
)

type Service struct {
	Name        string          `json:"name"`
	Tags        []string        `json:"tags"`
	Credentials json.RawMessage `json:"credentials"`
	InstanceID  string          `json:"instance_guid"`
}

type ObjectStoreCredentials struct {
	AccessKeyID     string `json:"access_key_id" validate:"required"`
	Bucket          string `json:"bucket" validate:"required"`
	Host            string `json:"host" validate:"required"`
	Region          string `json:"region" validate:"required"`
	SecretAccessKey string `json:"secret_access_key" validate:"required"`
	Uri             string `json:"uri"`
	Username        string `json:"username"`
}

// AMSCredentials are credentials from the legacy standalone authorization broker
type AMSCredentials struct {
	BundleURL   string                 `json:"bundle_url" validate:"required_without=ObjectStore"`
	ObjectStore ObjectStoreCredentials `json:"object_store" validate:"required_without=BundleURL"`
	URL         string                 `json:"url" validate:"required"`
	InstanceID  string                 `json:"instance_id"`
}

type IASCredentials struct {
	Certificate          string    `json:"certificate" validate:"required"`
	CertificateExpiresAt time.Time `json:"certificate_expires_at"`
	Clientid             string    `json:"clientid"`
	Domain               string    `json:"domain"`
	Domains              []string  `json:"domains"`
	Key                  string    `json:"key" validate:"required"`
	OsbURL               string    `json:"osb_url"`
	ProoftokenURL        string    `json:"prooftoken_url"`
	URL                  string    `json:"url"`
	ZoneUUID             string    `json:"zone_uuid"`
}

type UnifiedIdentityCredentials struct {
	IASCredentials
	AuthzURL         string                 `json:"authorization_url" validate:"required"`
	AuthzBundleURL   string                 `json:"authorization_bundle_url" validate:"required_without=AuthzObjectStore"`
	AuthzObjectStore ObjectStoreCredentials `json:"authorization_object_store" validate:"required_without=AuthzBundleURL"`
	AuthzInstanceID  string                 `json:"authorization_instance_id" validate:"required"`
}

func LoadServiceCredentials(log *libbuildpack.Logger, serviceName string) (credentials json.RawMessage, serviceInstanceId string, err error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]Service
	err = json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return json.RawMessage{}, "", fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
	}
	var rawAmsCreds []json.RawMessage
	if ups, ok := svcs["user-provided"]; ok {
		for i, up := range ups {
			for _, t := range up.Tags {
				if t == serviceName {
					log.Info("Detected user-provided %s service '%s", serviceName, ups[i].Name)
					rawAmsCreds = append(rawAmsCreds, ups[i].Credentials)
				}
			}
		}
	}
	var instanceID string
	for _, amsSvc := range svcs[serviceName] {
		instanceID = amsSvc.InstanceID
		rawAmsCreds = append(rawAmsCreds, amsSvc.Credentials)
	}
	if len(rawAmsCreds) != 1 {
		return json.RawMessage{}, "", fmt.Errorf("expect only one service (type %s or user-provided) but got %d", serviceName, len(rawAmsCreds))
	}
	return rawAmsCreds[0], instanceID, nil
}

func LoadIASClientCert(log *libbuildpack.Logger) (cert []byte, key []byte, err error) {
	iasCredsRaw, _, err := LoadServiceCredentials(log, "identity")
	if err != nil {
		return cert, key, err
	}
	var iasCreds IASCredentials
	err = json.Unmarshal(iasCredsRaw, &iasCreds)
	if err != nil {
		return cert, key, err
	}
	if iasCreds.Certificate == "" || iasCreds.Key == "" { // TODO: Provide option for {"credential_type":"X509_PROVIDED"}
		return cert, key, fmt.Errorf("identity service binding does not contain client certificate. Please use binding parameter {\"credential_type\":\"X509_GENERATED\"}")
	}

	return []byte(iasCreds.Certificate), []byte(iasCreds.Key), nil
}

func loadAMSCredentials(log *libbuildpack.Logger, cfg config) (AMSCredentials, error) {
	amsCreds, err := loadAMSCredsFromIAS(log)
	if err == nil {
		log.Debug("using authorization credentials embedded in identity service")
		return amsCreds, nil
	}
	log.Warning("no AMS credentials as part of identity service. Resorting to standalone authorization service broker")
	creds, id, err := LoadServiceCredentials(log, cfg.serviceName)
	if err != nil {
		return AMSCredentials{}, err
	}
	if err := json.Unmarshal(creds, &amsCreds); err != nil {
		return AMSCredentials{}, err
	}
	validate := validator.New()
	if err := validate.Struct(amsCreds); err != nil {
		return AMSCredentials{}, err
	}
	amsCreds.InstanceID = id // legacy mode, until all consumers have bindings with integrated instance_id
	return amsCreds, err
}

func loadAMSCredsFromIAS(log *libbuildpack.Logger) (AMSCredentials, error) {
	iasCredsRaw, _, err := LoadServiceCredentials(log, "identity")
	if err != nil {
		return AMSCredentials{}, err
	}
	var iasCreds UnifiedIdentityCredentials
	err = json.Unmarshal(iasCredsRaw, &iasCreds)
	if err != nil {
		return AMSCredentials{}, err
	}
	validate := validator.New()
	err = validate.Struct(iasCreds)
	return AMSCredentials{
		BundleURL:   iasCreds.AuthzBundleURL,
		ObjectStore: iasCreds.AuthzObjectStore,
		URL:         iasCreds.AuthzURL,
		InstanceID:  iasCreds.AuthzInstanceID,
	}, err
}
