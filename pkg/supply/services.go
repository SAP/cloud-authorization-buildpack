package supply

type Service struct {
	Name string `json:"name"`
	Plan string `json:"plan"`
}

type vcapServices map[string]interface{}

//struct {
//	Identity env.Identity `json:"identity",omitempty`
//	AMS	AMS `json:`
//}

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

type AMSCredentialsOld struct {
	UIURL       string                 `json:"ui_url"`
	ObjectStore ObjectStoreCredentials `json:"object_store"`
	Issuer      string                 `json:"value_help_certificate_issuer"`
	Subject     string                 `json:"value_help_certificate_subject"`
}

type AMSService struct {
	Service
	Credentials AMSCredentialsOld `json:"credentials"`
}
