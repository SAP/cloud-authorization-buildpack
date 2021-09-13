package supply

import "encoding/json"

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
	UIURL       string                 `json:"ui_url"`
	ObjectStore ObjectStoreCredentials `json:"object_store"`
	Issuer      string                 `json:"value_help_certificate_issuer"`
	Subject     string                 `json:"value_help_certificate_subject"`
	URL         string                 `json:"url"`
}
