package supply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/mitchellh/mapstructure"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"
)

type Stager interface {
	//TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/stager.go
	BuildDir() string
	DepDir() string
	DepsIdx() string
	DepsDir() string
}

type Manifest interface {
	//TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/manifest.go
	AllDependencyVersions(string) []string
	DefaultVersion(string) (libbuildpack.Dependency, error)
}

type Installer interface {
	//TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/installer.go
	InstallDependency(libbuildpack.Dependency, string) error
	InstallOnlyVersion(string, string) error
}

type Command interface {
	//TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/command.go
	Execute(string, io.Writer, io.Writer, string, ...string) error
	Output(dir string, program string, args ...string) (string, error)
}

type Supplier struct {
	Manifest     Manifest
	Installer    Installer
	Stager       Stager
	Command      Command
	Log          *libbuildpack.Logger
	BuildpackDir string
}

type Cloudfoundry struct {
	SidecarFor []string `yaml:"sidecar_for" json:"sidecar_for"`
}

type Platforms struct {
	Cloudfoundry Cloudfoundry `yaml:"cloudfoundry" json:"cloudfoundry"`
}

type Limits struct {
	Memory int `yaml:"memory" json:"memory"`
}

type Process struct {
	Type      string    `yaml:"type" json:"type"`
	Command   string    `yaml:"command" json:"command"`
	Platforms Platforms `yaml:"platforms" json:"platforms"`
	Limits    Limits    `yaml:"limits" json:"limits"`
}
type LaunchData struct {
	Processes []Process `yaml:"processes" json:"processes"`
}

func (s *Supplier) Run() error {
	if err := s.supplyExecResource("opa"); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.supplyExecResource("start_opa.sh"); err != nil {
		return fmt.Errorf("could not supply start_opa.sh: %w", err)
	}
	ams, err := s.loadAMSService()
	if err != nil {
		return fmt.Errorf("could not load AMSService: %w", err)
	}
	if err := s.writeLaunchConfig(); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(ams.Credentials.ObjectStore); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeEnvFile(ams); err != nil {
		return fmt.Errorf("could not write env file: %w", err)
	}

	return nil
}

type S3Signing struct {
	AWSEnvCreds interface{} `json:"environment_credentials,omitempty"`
}

type Credentials struct {
	S3Signing S3Signing `json:"s3_signing,omitempty"`
}

type RestConfig struct {
	URL         string      `json:"url"`
	Credentials Credentials `json:"credentials"`
}

type Config struct {
	Bundles  map[string]*bundle.Source `json:"bundles"`
	Services map[string]RestConfig     `json:"services"`
}

func (s *Supplier) writeEnvFile(ams AMSService) error {
	s.Log.Info("writing env file..")
	var b bytes.Buffer
	b.WriteString("export OPA_URL=http://localhost:9888")

	b.WriteString(fmt.Sprint("export AWS_ACCESS_KEY_ID=", ams.Credentials.ObjectStore.AccessKeyID))

	dirPath := path.Join(s.Stager.BuildDir(), ".profile.d")
	err := os.Mkdir(dirPath, 0755)
	if err != nil {
		return fmt.Errorf("could not create profile directory '%s': %w", dirPath, err)
	}

	filePath := path.Join(dirPath, "0000_opa_env.sh")
	err = os.WriteFile(filePath, b.Bytes(), 0755)
	if err != nil {
		return fmt.Errorf("could not write file to '%s': %w", filePath, err)
	}
	return nil
}

func (s *Supplier) writeOpaConfig(osCreds ObjectStoreCredentials) error {
	s.Log.Info("writing opa config..")
	serviceKey := "s3"
	bundles := make(map[string]*bundle.Source)
	bundles["SAP"] = &bundle.Source{
		Config: download.Config{
			Polling: download.PollingConfig{
				MinDelaySeconds: newInt64P(10),
				MaxDelaySeconds: newInt64P(20),
			},
		},
		Service:  serviceKey,
		Resource: "SAP.tar.gz",
	}
	services := make(map[string]RestConfig)
	services[serviceKey] = RestConfig{
		URL:         fmt.Sprintf("https://%s/%s", osCreds.Host, osCreds.Bucket),
		Credentials: Credentials{S3Signing{AWSEnvCreds: struct{}{}}},
	}

	cfg := Config{
		Bundles:  bundles,
		Services: services,
	}
	opaConfigBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("could not marshal bundle config: %w", err)
	}
	log.Println(string(opaConfigBytes))
	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	err = os.WriteFile(filePath, opaConfigBytes, 0644)
	if err != nil {
		return fmt.Errorf("could not write file to '%s': %w", filePath, err)
	}
	return nil
}

func (s *Supplier) writeLaunchConfig() error {
	s.Log.Info("writing launch.yml..")
	launchData := LaunchData{
		[]Process{
			{
				Type:      "opa",
				Command:   path.Join(s.Stager.DepDir(), "start_opa.sh"),
				Platforms: Platforms{Cloudfoundry{[]string{"web"}}},
				Limits:    Limits{100},
			},
		},
	}
	launchDataBytes, err := json.Marshal(launchData)
	if err != nil {
		return fmt.Errorf("could not marshal process config: %w", err)
	}
	filePath := path.Join(s.Stager.DepDir(), "launch.yml")
	err = os.WriteFile(filePath, launchDataBytes, 0644)
	if err != nil {
		return fmt.Errorf("could not write file to '%s': %w", filePath, err)
	}
	return nil
}

func (s *Supplier) loadAMSService() (AMSService, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string]interface{}
	err := json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return AMSService{}, fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
	}
	var ams []AMSService
	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &ams,
		TagName: "json",
	})
	if err != nil {
		return AMSService{}, fmt.Errorf("errr creating decoder: %w", err)
	}
	err = d.Decode(svcs["authorization"])
	if err != nil {
		return AMSService{}, fmt.Errorf("could not decode 'authorization' service: %w", err)
	}
	if len(ams) != 1 {
		return AMSService{}, fmt.Errorf("expect only one AMS service, but got %d", len(ams))
	}
	return ams[0], nil
}

func (s *Supplier) supplyExecResource(resource string) error {
	src, err := os.Open(path.Join(s.BuildpackDir, "resources", resource))
	if err != nil {
		return fmt.Errorf("could not read resource: %w", err)
	}
	dst, err := os.OpenFile(path.Join(s.Stager.DepDir(), resource), os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("could not create file for resource: %w", err)
	}
	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("could not copy resource: %w", err)
	}
	return nil
}
