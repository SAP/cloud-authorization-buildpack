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
}

type amsDataDeprecated struct {
	Root string `json:"root"`
}

type VcapApplication struct {
	ApplicationName string `json:"application_name"`
}

func LoadVcapApplication(log *libbuildpack.Logger) VcapApplication {
	vcapStr, vcapSet := os.LookupEnv("VCAP_APPLICATION")
	var result VcapApplication
	if vcapSet {
		err := json.Unmarshal([]byte(vcapStr), &result)
		if err != nil {
			log.Error("error parsing VCAP_APPLICATION value %s : %v", vcapStr, err)
		}
	}
	return result
}

func LoadBuildpackConfig(log *libbuildpack.Logger) (Config, error) {
	// Deprecated compatibility coding to support AMS_DATA for now.
	// (AMS_DATA.serviceName is intentionally ignored: never officially supported.)
	amsData, amsDataSet := os.LookupEnv("AMS_DATA")
	if amsDataSet {
		log.Warning("the environment variable AMS_DATA is deprecated. Please use $AMS_DCL_ROOT to provide Base DCL application (see https://github.com/SAP/cloud-authorization-buildpack/blob/main/README.md#base-policy-upload)")
		var amsD amsDataDeprecated
		err := json.Unmarshal([]byte(amsData), &amsD)
		return Config{
			Root:         amsD.Root,
			ShouldUpload: amsD.Root != "",
		}, err
	}
	// End of deprecated coding

	dclRoot := os.Getenv("AMS_DCL_ROOT")
	shouldUpload := dclRoot != ""
	if !shouldUpload {
		log.Warning("this app will upload no authorization data (AMS_DCL_ROOT empty or not set)")
	}
	return Config{
		Root:         dclRoot,
		ShouldUpload: shouldUpload,
	}, nil
}
