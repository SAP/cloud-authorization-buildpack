package supply

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/SAP/cloud-authorization-buildpack/pkg/archive"
	"github.com/SAP/cloud-authorization-buildpack/pkg/client"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"
)

const ServiceName = "authorization"

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
	amsCreds, err := s.loadAMSCredentials(s.Log, cfg)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	if err := s.writeLaunchConfig(); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(amsCreds.ObjectStore); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeProfileDFile(amsCreds); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}
	if err := s.uploadAuthzData(amsCreds, cfg); err != nil {
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

func (s *Supplier) writeProfileDFile(amsCreds AMSCredentials) error {
	s.Log.Info("writing profileD file..")
	opaPort := 9888
	//TODO: I removed setting log level to DEbug, because why do it
	values := map[string]string{
		"AWS_ACCESS_KEY_ID":     amsCreds.ObjectStore.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": amsCreds.ObjectStore.SecretAccessKey,
		"AWS_REGION":            amsCreds.ObjectStore.Region,
		"opa_binary":            path.Join(s.Stager.DepDir(), "opa"),
		"opa_config":            path.Join(s.Stager.DepDir(), "opa_config.yml"),
		"OPA_URL":               fmt.Sprintf("http://localhost:%d/", opaPort),
		"OPA_PORT":              strconv.Itoa(opaPort),
		"ADC_URL":               fmt.Sprintf("http://localhost:%d/", opaPort),
	}
	var b strings.Builder
	for k, v := range values {
		b.WriteString(fmt.Sprintf("export %s=%s\n", k, v))
	}
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

func (s *Supplier) loadAMSCredentials(log *libbuildpack.Logger, cfg config) (AMSCredentials, error) {
	svcsString := os.Getenv("VCAP_SERVICES")
	var svcs map[string][]Service
	err := json.Unmarshal([]byte(svcsString), &svcs)
	if err != nil {
		return AMSCredentials{}, fmt.Errorf("could not unmarshal VCAP_SERVICES: %w", err)
	}
	var rawAmsCreds []json.RawMessage
	if ups, ok := svcs["user-provided"]; ok {
		for i, up := range ups {
			for _, t := range up.Tags {
				if t == ServiceName {
					log.Info("Detected user-provided authorization service '%s", ups[i].Name)
					rawAmsCreds = append(rawAmsCreds, ups[i].Credentials)
				}
			}
		}
	}
	for _, amsSvc := range svcs[cfg.serviceName] {
		rawAmsCreds = append(rawAmsCreds, amsSvc.Credentials)
	}
	if len(rawAmsCreds) != 1 {
		return AMSCredentials{}, fmt.Errorf("expect only one AMS service (type %s or user-provided) but got %d", cfg.serviceName, len(rawAmsCreds))
	}
	var amsCreds AMSCredentials
	err = json.Unmarshal(rawAmsCreds[0], &amsCreds)
	return amsCreds, err
}

func (s *Supplier) supplyExecResource(resource string) error {
	return libbuildpack.CopyFile(path.Join(s.BuildpackDir, "resources", resource), path.Join(s.Stager.DepDir(), resource))
}

type config struct {
	root        string
	serviceName string
}

func (s *Supplier) uploadAuthzData(amsCreds AMSCredentials, cfg config) error {
	s.Log.Info("creating policy archive..")

	if cfg.root == "" {
		s.Log.Warning("this app will upload no authorization data. AMS_DCL_ROOT empty or not set")
		return nil
	}
	buf, err := archive.CreateArchive(s.Log, path.Join(s.Stager.BuildDir(), cfg.root))
	if err != nil {
		return fmt.Errorf("could not create policy bundle.tar.gz: %w", err)
	}
	u, err := url.Parse(amsCreds.URL)
	if err != nil {
		return fmt.Errorf("invalid AMS URL ('%s'): %w", amsCreds.URL, err)
	}
	u.Path = path.Join(u.Path, "/sap/ams/v1/bundles/SAP.tar.gz")
	r, err := http.NewRequest(http.MethodPost, u.String(), buf)
	if err != nil {
		return fmt.Errorf("could not create bundle upload request %w", err)
	}
	r.Header.Set("Content-Type", "application/gzip")
	resp, err := s.AMSClient.Do(r)
	if err != nil {
		return fmt.Errorf("bundle upload request unsuccessful: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("unexpected response: status(%s) body(%s)", resp.Status, resp.Body)
	}
	return nil
}

func (s *Supplier) loadBuildpackConfig() (config, error) {
	_, amsDataSet := os.LookupEnv("AMS_DATA")
	if amsDataSet {
		return config{}, fmt.Errorf("the environment variable AMS_DATA is not supported anymore. Please use $AMS_DCL_ROOT to provide Base DCL application")
	}
	serviceName := os.Getenv("AMS_SERVICE")
	if serviceName == "" {
		serviceName = ServiceName
	}
	return config{
		serviceName: serviceName,
		root:        os.Getenv("AMS_DCL_ROOT"),
	}, nil
}
