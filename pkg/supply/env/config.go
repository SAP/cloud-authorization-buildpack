package env

import (
	"encoding/json"
	"os"

	"github.com/cloudfoundry/libbuildpack"
)

const ServiceName = "authorization"

type Config struct {
	Root         string
	ServiceName  string
	ShouldUpload bool
	LogLevel     string
	Port         int
}

type amsDataDeprecated struct {
	Root string `json:"root"`
}

func LoadBuildpackConfig(log *libbuildpack.Logger) (Config, error) {
	serviceName := os.Getenv("AMS_SERVICE")
	if serviceName == "" {
		serviceName = ServiceName
	}

	// Deprecated compatibility coding to support AMS_DATA for now (AMS_DATA.serviceNname will be ignored, because its not supposed to be supported by stakeholders)
	amsData, amsDataSet := os.LookupEnv("AMS_DATA")
	if amsDataSet {
		log.Warning("the environment variable AMS_DATA is deprecated. Please use $AMS_DCL_ROOT to provide Base DCL application (see https://github.com/SAP/cloud-authorization-buildpack/blob/master/README.md#base-policy-upload)")
		var amsD amsDataDeprecated
		err := json.Unmarshal([]byte(amsData), &amsD)
		return Config{
			ServiceName:  serviceName,
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
		ServiceName:  serviceName,
		Root:         dclRoot,
		ShouldUpload: shouldUpload,
		LogLevel:     logLevel,
		Port:         9888,
	}, nil
}
