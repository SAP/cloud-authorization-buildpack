package env

import (
	"encoding/json"
	"os"

	"github.com/cloudfoundry/libbuildpack"
)

const HeaderInstanceID = "X-Ams-Instance-Id"

type Config struct {
	Root         string
	ShouldUpload bool
	LogLevel     string
	Port         int
}

type amsDataDeprecated struct {
	Root string `json:"root"`
}

type VcapApplication struct {
	ApplicationID   string   `json:"application_id"`
	ApplicationName string   `json:"application_name"`
	ApplicationUris []string `json:"application_uris"`
	CfAPI           string   `json:"cf_api"`
	Limits          struct {
		Fds int `json:"fds"`
	} `json:"limits"`
	Name             string   `json:"name"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationName string   `json:"organization_name"`
	SpaceID          string   `json:"space_id"`
	SpaceName        string   `json:"space_name"`
	Uris             []string `json:"uris"`
	Users            any      `json:"users"`
}

func LoadVcapApplication(log *libbuildpack.Logger) (VcapApplication, error) {
	vcapStr, vcapSet := os.LookupEnv("VCAP_APPLICATION")
	var result VcapApplication
	if vcapSet {
		err := json.Unmarshal([]byte(vcapStr), &result)
		if err != nil {
			log.Error("error parsing VCAP_APPLICATION value %s : %v", vcapStr, err)
		}
	}
	return result, nil
}

func LoadBuildpackConfig(log *libbuildpack.Logger) (Config, error) {
	// Deprecated compatibility coding to support AMS_DATA for now (AMS_DATA.serviceNname will be ignored, because its not supposed to be supported by stakeholders)
	amsData, amsDataSet := os.LookupEnv("AMS_DATA")
	if amsDataSet {
		log.Warning("the environment variable AMS_DATA is deprecated. Please use $AMS_DCL_ROOT to provide Base DCL application (see https://github.com/SAP/cloud-authorization-buildpack/blob/main/README.md#base-policy-upload)")
		var amsD amsDataDeprecated
		err := json.Unmarshal([]byte(amsData), &amsD)
		return Config{
			Root:         amsD.Root,
			ShouldUpload: amsD.Root != "",
			LogLevel:     "info",
			Port:         9888,
		}, err
	}
	// End of Deprecated coding

	dclRoot := os.Getenv("AMS_DCL_ROOT")
	shouldUpload := dclRoot != ""
	if !shouldUpload {
		log.Warning("this app will upload no authorization data (AMS_DCL_ROOT empty or not set)")
	}
	logLevel := os.Getenv("AMS_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "error"
	}
	return Config{
		Root:         dclRoot,
		ShouldUpload: shouldUpload,
		LogLevel:     logLevel,
		Port:         9888,
	}, nil
}
