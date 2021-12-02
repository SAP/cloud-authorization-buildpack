package supply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"

	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
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

	// after remove of AMS_DATA, err and logger con be removed from this method
	cfg, err := s.loadBuildpackConfig(s.Log)
	if err != nil {
		return fmt.Errorf("could not load buildpack config: %w", err)
	}
	amsCreds, err := loadAMSCredentials(s.Log, cfg)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	if err := s.supplyOPABinary(); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.writeLaunchConfig(cfg); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(amsCreds); err != nil {
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

type ClientTLS struct {
	Cert string `json:"cert,omitempty"`
	Key  string `json:"private_key,omitempty"`
}

type Credentials struct {
	S3Signing *S3Signing `json:"s3_signing,omitempty"` // old direct s3 bundle access
	ClientTLS *ClientTLS `json:"client_tls,omitempty"` // new storage gateway bundle access
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
		"OPA_URL": fmt.Sprintf("http://localhost:%d/", cfg.port),
		"ADC_URL": fmt.Sprintf("http://localhost:%d/", cfg.port),
	}

	if len(amsCreds.BundleURL) == 0 {
		values["AWS_ACCESS_KEY_ID"] = amsCreds.ObjectStore.AccessKeyID
		values["AWS_SECRET_ACCESS_KEY"] = amsCreds.ObjectStore.SecretAccessKey
		values["AWS_REGION"] = amsCreds.ObjectStore.Region
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

func (s *Supplier) writeOpaConfig(cred AMSCredentials) error {
	s.Log.Info("writing opa config..")

	var cfg OPAConfig
	if len(cred.BundleURL) != 0 {
		cfg = s.createStorageGatewayConfig(cred)
	} else {
		cfg = s.createDirectS3OpaConfig(*cred.ObjectStore)
	}

	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	bCfg, _ := json.Marshal(cfg)
	s.Log.Debug("OPA config: '%s'", string(bCfg))
	return libbuildpack.NewJSON().Write(filePath, cfg)
}

func (s *Supplier) createDirectS3OpaConfig(osCreds ObjectStoreCredentials) OPAConfig {
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
		Credentials: Credentials{S3Signing: &S3Signing{AWSEnvCreds: struct{}{}}},
	}

	return OPAConfig{
		Bundles:  bundles,
		Services: services,
	}
}

func (s *Supplier) createStorageGatewayConfig(cred AMSCredentials) OPAConfig {
	serviceKey := "bundle_storage"
	bundles := make(map[string]*bundle.Source)
	bundles[cred.InstanceID] = &bundle.Source{
		Config: download.Config{
			Polling: download.PollingConfig{
				MinDelaySeconds: newInt64P(10),
				MaxDelaySeconds: newInt64P(20),
			},
		},
		Service:  serviceKey,
		Resource: cred.InstanceID + ".tar.gz",
	}
	services := make(map[string]RestConfig)
	services[serviceKey] = RestConfig{
		URL: cred.BundleURL,
		Credentials: Credentials{ClientTLS: &ClientTLS{
			Cert: "/home/vcap/deps/0/ias.crt",
			Key:  "/home/vcap/deps/0/ias.key",
		}},
	}

	return OPAConfig{
		Bundles:  bundles,
		Services: services,
	}
}

func (s *Supplier) writeLaunchConfig(cfg config) error {
	s.Log.Info("writing launch.yml..")
	cmd := fmt.Sprintf(
		"\"$DEPS_DIR/%s\" run -s -c \"$DEPS_DIR/%s\" -l '%s' -a '127.0.0.1:%d' --skip-version-check",
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

func (s *Supplier) supplyOPABinary() error {
	opaDep, err := s.Manifest.DefaultVersion("opa")
	if err != nil {
		return err
	}
	if err = s.Installer.InstallDependency(opaDep, s.Stager.DepDir()); err != nil {
		return fmt.Errorf("couldn't install OPA dependency: %w", err)
	}
	// The packager overwrites the permissions, so we need to make it executable again
	return os.Chmod(path.Join(s.Stager.DepDir(), opaDep.Name), 0755)
}

type config struct {
	root         string
	serviceName  string
	shouldUpload bool
	logLevel     string
	port         int
}
type amsDataDeprecated struct {
	Root string `json:"root"`
}

func (s *Supplier) loadBuildpackConfig(log *libbuildpack.Logger) (config, error) {

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
		return config{
			serviceName:  serviceName,
			root:         amsD.Root,
			shouldUpload: amsD.Root != "",
			logLevel:     "info",
			port:         9888,
		}, err

	}
	// End of Deprecated coding

	dclRoot := os.Getenv("AMS_DCL_ROOT")
	shouldUpload := dclRoot != ""
	if !shouldUpload {
		s.Log.Warning("this app will upload no authorization data (AMS_DCL_ROOT empty or not set)")
	}
	logLevel := os.Getenv("AMS_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "error"
	}
	return config{
		serviceName:  serviceName,
		root:         dclRoot,
		shouldUpload: shouldUpload,
		logLevel:     logLevel,
		port:         9888,
	}, nil
}
