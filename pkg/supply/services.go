package supply

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry/libbuildpack"
)

type Service struct {
	Name        string          `json:"name"`
	Tags        []string        `json:"tags"`
	Credentials json.RawMessage `json:"credentials"`
}

type ObjectStoreCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	Bucket          string `json:"bucket"`
	Host            string `json:"host"`
	Region          string `json:"region"`
	SecretAccessKey string `json:"secret_access_key"`
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
	UIURL     string `json:"ui_url"`
	BundleURL string `json:"bundle_url"`
	Issuer    string `json:"value_help_certificate_issuer"`
	Subject   string `json:"value_help_certificate_subject"`
	URL       string `json:"url"`
}

type IASCredentials struct {
	Certificate          string    `json:"certificate"`
	CertificateExpiresAt time.Time `json:"certificate_expires_at"`
	Clientid             string    `json:"clientid"`
	Domain               string    `json:"domain"`
	Domains              []string  `json:"domains"`
	Key                  string    `json:"key"`
	OsbURL               string    `json:"osb_url"`
	ProoftokenURL        string    `json:"prooftoken_url"`
	URL                  string    `json:"url"`
	ZoneUUID             string    `json:"zone_uuid"`
}

func LoadServiceCredentials(log *libbuildpack.Logger, serviceName string) (json.RawMessage, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]Service
	err := json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return json.RawMessage{}, fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
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
	for _, amsSvc := range svcs[serviceName] {
		rawAmsCreds = append(rawAmsCreds, amsSvc.Credentials)
	}
	if len(rawAmsCreds) != 1 {
		return json.RawMessage{}, fmt.Errorf("expect only one service (type %s or user-provided) but got %d", serviceName, len(rawAmsCreds))
	}
	return rawAmsCreds[0], nil
}
