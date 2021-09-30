package supply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
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
	Uploader     uploader.Uploader
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
	cfg, err := s.loadBuildpackConfig()
	if err != nil {
		return fmt.Errorf("could not load buildpack config: %w", err)
	}
	amsCreds, err := s.loadAMSCredentials(s.Log, cfg)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	if err := s.supplyOPABinary(); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.writeLaunchConfig(cfg); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(amsCreds.ObjectStore); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeProfileDFile(cfg, amsCreds); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}
	if cfg.shouldUpload {
		if err := s.Uploader.Upload(path.Join(s.Stager.BuildDir(), cfg.root), amsCreds.URL); err != nil {
			return fmt.Errorf("could not upload authz data: %w", err)
		}
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

func (s *Supplier) writeProfileDFile(cfg config, amsCreds AMSCredentials) error {
	s.Log.Info("writing profileD file..")
	values := map[string]string{
		"AWS_ACCESS_KEY_ID":     amsCreds.ObjectStore.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": amsCreds.ObjectStore.SecretAccessKey,
		"AWS_REGION":            amsCreds.ObjectStore.Region,
		"OPA_URL":               fmt.Sprintf("http://localhost:%d/", cfg.port),
		"ADC_URL":               fmt.Sprintf("http://localhost:%d/", cfg.port),
	}
	var b bytes.Buffer
	for k, v := range values {
		b.WriteString(fmt.Sprintf("export %s=%s\n", k, v))
	}
	// We do not use libbuildpack.WriteProfileD, because the copy mechanism form deps_dir to build_dir
	// does not work for sidecar buildpacks (deps_dir/bin and deps_dir/profileD)
	if err := os.MkdirAll(s.Stager.ProfileDir(), 0755); err != nil {
		return fmt.Errorf("couldn't create profile dir: %w", err)
	}
	return os.WriteFile(path.Join(s.Stager.ProfileDir(), "0000_opa_env.sh"), b.Bytes(), 0755)
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
	bCfg, _ := json.Marshal(cfg)
	s.Log.Debug("OPA config: '%s'", string(bCfg))
	return libbuildpack.NewJSON().Write(filePath, cfg)
}

func (s *Supplier) writeLaunchConfig(cfg config) error {
	s.Log.Info("writing launch.yml..")
	cmd := fmt.Sprintf(
		"\"$DEPS_DIR/%s\" run -s -c \"$DEPS_DIR/%s\" -l '%s' -a '[]:%d'",
		path.Join(s.Stager.DepsIdx(), "opa"),
		path.Join(s.Stager.DepsIdx(), "opa_config.yml"),
		cfg.logLevel,
		9888)
	s.Log.Debug("OPA start command: '%s'", cmd)
	launchData := LaunchData{
		[]Process{
			{
				Type:      "opa",
				Command:   cmd,
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

func (s *Supplier) supplyOPABinary() error {
	opaDep, err := s.Manifest.DefaultVersion("opa")
	if err != nil {
		return err
	}
	return s.Installer.InstallDependency(opaDep, path.Join(s.Stager.DepDir()))
}

type config struct {
	root         string
	serviceName  string
	shouldUpload bool
	logLevel     string
	port         int
}

func (s *Supplier) loadBuildpackConfig() (config, error) {
	_, amsDataSet := os.LookupEnv("AMS_DATA")
	if amsDataSet {
		return config{}, fmt.Errorf("the environment variable AMS_DATA is not supported anymore. Please use $AMS_DCL_ROOT to provide Base DCL application (see https://github.com/SAP/cloud-authorization-buildpack/blob/master/README.md#base-policy-upload)")
	}
	serviceName := os.Getenv("AMS_SERVICE")
	if serviceName == "" {
		serviceName = ServiceName
	}
	dclRoot := os.Getenv("AMS_DCL_ROOT")
	shouldUpload := dclRoot != ""
	if !shouldUpload {
		s.Log.Warning("this app will upload no authorization data (AMS_DCL_ROOT empty or not set)")
	}
	logLevel := os.Getenv("AMS_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	return config{
		serviceName:  serviceName,
		root:         dclRoot,
		shouldUpload: shouldUpload,
		logLevel:     logLevel,
		port:         9888,
	}, nil
}
