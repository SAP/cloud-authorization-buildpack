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

//type AMSCredentials struct{
//	UIURL       string                 `json:"authorization_ui_url"`
//	ObjectStore ObjectStoreCredentials `json:"authorization_object_store"`
//	Issuer      string                 `json:"authorization_value_help_certificate_issuer"`
//	Subject     string                 `json:"authorization_value_help_certificate_subject"`
//}

// This is the old way of marshaling creds
type AMSCredentials struct {
	UIURL      string `json:"ui_url"`
	BundleURL  string `json:"bundle_url"`
	ObjectStore ObjectStoreCredentials `json:"object_store"`
	Issuer     string `json:"value_help_certificate_issuer"`
	Subject    string `json:"value_help_certificate_subject"`
	URL        string `json:"url"`
	InstanceID string `json:"instance_guid"`
}

type IASCredentials struct {
	Certificate          string                 `json:"certificate" validate:"required"`
	CertificateExpiresAt time.Time              `json:"certificate_expires_at"`
	Clientid             string                 `json:"clientid"`
	Domain               string                 `json:"domain"`
	Domains              []string               `json:"domains"`
	Key                  string                 `json:"key" validate:"required"`
	OsbURL               string                 `json:"osb_url"`
	ProoftokenURL        string                 `json:"prooftoken_url"`
	URL                  string                 `json:"url"`
	ZoneUUID             string                 `json:"zone_uuid"`
	AuthzURL             string                 `json:"authorization_url" validate:"required,url"`
	AuthzObjectStore     ObjectStoreCredentials `json:"authorization_object_store" validate:"required"`
	AuthzUIURL           string                 `json:"authorization_ui_url" validate:"url,required"`
}

func LoadServiceCredentials(log *libbuildpack.Logger, serviceName string) (json.RawMessage, string, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]Service
	err := json.Unmarshal([]byte(svcsString), &svcs)
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

func loadAMSCredsFromIAS(log *libbuildpack.Logger) (AMSCredentials, error) {
	iasCredsRaw, err := LoadServiceCredentials(log, "identity")
	if err != nil {
		return AMSCredentials{}, err
	}
	var iasCreds IASCredentials
	err = json.Unmarshal(iasCredsRaw, &iasCreds)
	if err != nil {
		return AMSCredentials{}, err
	}
	validate := validator.New()
	err = validate.Struct(iasCreds)
	return AMSCredentials{
		UIURL:       iasCreds.AuthzUIURL,
		ObjectStore: iasCreds.AuthzObjectStore,
		URL:         iasCreds.AuthzURL,
	}, err
}
