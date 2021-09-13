package supply

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/SAP/cloud-authorization-buildpack/pkg/client"
	"github.com/SAP/cloud-authorization-buildpack/pkg/compressor"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"
)

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
	Stager       *libbuildpack.Stager
	Command      Command
	Log          *libbuildpack.Logger
	BuildpackDir string
	AMSClient    client.AMSClient
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
	s.Log.BeginStep("Supplying OPA")
	if err := s.supplyExecResource("opa"); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.supplyExecResource("start_opa.sh"); err != nil {
		return fmt.Errorf("could not supply start_opa.sh: %w", err)
	}
	cfg, err := s.loadBuildpackConfig()
	if err != nil {
		return fmt.Errorf("could not load buildpack config: %w", err)
	}
	ams, err := s.loadAMSService(cfg)
	if err != nil {
		return fmt.Errorf("could not load AMSService: %w", err)
	}
	if err := s.writeLaunchConfig(); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(ams.Credentials.ObjectStore); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeProfileDFile(ams); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}
	if err := s.uploadAuthzData(ams, cfg); err != nil {
		return fmt.Errorf("could not upload authz data: %w", err)
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

type OPAConfig struct {
	Bundles  map[string]*bundle.Source `json:"bundles"`
	Services map[string]RestConfig     `json:"services"`
}

func (s *Supplier) writeProfileDFile(ams AMSService) error {
	s.Log.Info("writing profileD file..")
	var b strings.Builder
	b.WriteString("export OPA_URL=http://localhost:9888\n")
	b.WriteString(fmt.Sprintf("export AWS_ACCESS_KEY_ID=%s\n", ams.Credentials.ObjectStore.AccessKeyID))

	return s.Stager.WriteProfileD("0000_opa_env.sh", b.String())
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

	cfg := OPAConfig{
		Bundles:  bundles,
		Services: services,
	}

	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	return libbuildpack.NewJSON().Write(filePath, cfg)
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
	filePath := path.Join(s.Stager.DepDir(), "launch.yml")
	return libbuildpack.NewJSON().Write(filePath, launchData)
}

func (s *Supplier) loadAMSService(cfg Config) (AMSService, error) {
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
	err = d.Decode(svcs[cfg.ServiceName])
	if err != nil {
		return AMSService{}, fmt.Errorf("could not decode 'authorization' service: %w", err)
	}
	if len(ams) != 1 {
		return AMSService{}, fmt.Errorf("expect only one service of type %s, but got %d", cfg.ServiceName, len(ams))
	}
	return ams[0], nil
}

func (s *Supplier) supplyExecResource(resource string) error {
	return libbuildpack.CopyFile(path.Join(s.BuildpackDir, "resources", resource), path.Join(s.Stager.DepDir(), resource))
}

type Config struct {
	Root        string   `json:"root" validate:"required"`
	Directories []string `json:"directories" validate:"required,gt=0,dive,required"`
	ServiceName string   `json:"service_name"`
}

func (s *Supplier) uploadAuthzData(ams AMSService, cfg Config) error {
	amsDataStr := os.Getenv("AMS_DATA")
	if amsDataStr == "" {
		s.Log.Warning("this app will upload no authorization data (AMS_DATA empty or not set)")
		return nil
	}
	buf, err := compressor.CreateArchive(path.Join(s.BuildpackDir, cfg.Root), cfg.Directories)
	if err != nil {
		return fmt.Errorf("could not create policy bundle.tar.gz: %w", err)
	}
	url, err := url.Parse(ams.URL)
	if err != nil {
		return fmt.Errorf("invalid AMS URL ('%s'): %w", ams.URL, err)
	}
	url.Path = path.Join(url.Path, "/sap/ams/v1/bundles/SAP.tar.gz")
	r, err := http.NewRequest(http.MethodPost, url.String(), buf)
	if err != nil {
		return fmt.Errorf("could not create bundle upload request %w", err)
	}
	r.Header.Set("Content-Type", "application/gzip")
	resp, err := s.AMSClient.Do(r)
	if err != nil {
		return fmt.Errorf("bundle upload request unsuccessful: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected response status: '%s'", resp.Status)
	}

	return nil
}

func (s *Supplier) loadBuildpackConfig() (Config, error) {
	cfgStr := os.Getenv("AMS_DATA")
	if cfgStr == "" {
		s.Log.Warning("this app will upload no authorization data (AMS_DATA empty or not set)")
		return Config{ServiceName: "authorization"}, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(cfgStr), &cfg); err != nil {
		return Config{}, fmt.Errorf("could not unmarshal AMS_DATA: %w", err)
	}
	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("invalid AMS_DATA: %w", err)
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "authorization"
	}
	return cfg, nil
}
