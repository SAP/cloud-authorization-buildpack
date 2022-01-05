package supply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/env"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/services"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"

	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
)

type Manifest interface {
	// TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/manifest.go
	AllDependencyVersions(string) []string
	DefaultVersion(string) (libbuildpack.Dependency, error)
}

type Installer interface {
	// TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/installer.go
	InstallDependency(libbuildpack.Dependency, string) error
	InstallOnlyVersion(string, string) error
}

type Command interface {
	// TODO: See more options at https://github.com/cloudfoundry/libbuildpack/blob/master/command.go
	Execute(string, io.Writer, io.Writer, string, ...string) error
	Output(dir string, program string, args ...string) (string, error)
}

type Supplier struct {
	Manifest      Manifest
	Installer     Installer
	Stager        *libbuildpack.Stager
	Command       Command
	Log           *libbuildpack.Logger
	BuildpackDir  string
	UploadBuilder func(log *libbuildpack.Logger, cert, key string) (uploader.Uploader, error)
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
	cfg, err := env.LoadBuildpackConfig(s.Log)
	if err != nil {
		return fmt.Errorf("could not load buildpack Config: %w", err)
	}
	amsCreds, err := services.LoadAMSCredentials(s.Log, cfg, services.MegacliteLoader{}, services.IdentityLoader{}, services.AuthorizationLoader{})
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	if err := s.addTLSCreds(&amsCreds); err != nil {
		return fmt.Errorf("could not add TLS credentials: %w", err)
	}
	if err := s.supplyOPABinary(); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.writeLaunchConfig(cfg); err != nil {
		return fmt.Errorf("could not write launch Config: %w", err)
	}
	if err := s.writeOpaConfig(amsCreds); err != nil {
		return fmt.Errorf("could not write opa Config: %w", err)
	}
	if err := s.writeProfileDFile(cfg, amsCreds); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}

	if cfg.ShouldUpload {
		if err := s.upload(cfg, amsCreds); err != nil {
			return fmt.Errorf("error uploading policies: %w", err)
		}
	}
	return nil
}

func (s *Supplier) addTLSCreds(amsCreds *services.AMSCredentials) error {
	if amsCreds.InstanceID == "dwc-megaclite-ams-instance-id" {
		return nil
	}
	cert, key, err := services.LoadIASClientCert(s.Log)
	if err != nil {
		return err
	}
	err = os.WriteFile(path.Join(s.Stager.DepDir(), "ias.crt"), cert, 0600)
	if err != nil {
		return fmt.Errorf("unable to write IAS client certificate: %s", err)
	}
	err = os.WriteFile(filepath.Join(s.Stager.DepDir(), "ias.key"), key, 0600)
	if err != nil {
		return fmt.Errorf("unable to write IAS client key: %s", err)
	}
	amsCreds.CertPath = path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.crt")
	amsCreds.KeyPath = path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.key")
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
	Plugins  map[string]bool           `json:"plugins,omitempty"`
}

func (s *Supplier) writeProfileDFile(cfg env.Config, amsCreds services.AMSCredentials) error {
	s.Log.Info("writing profileD file..")
	values := map[string]string{
		"OPA_URL": fmt.Sprintf("http://localhost:%d/", cfg.Port),
		"ADC_URL": fmt.Sprintf("http://localhost:%d/", cfg.Port),
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

func (s *Supplier) writeOpaConfig(cred services.AMSCredentials) error {
	s.Log.Info("writing opa Config..")

	var cfg OPAConfig
	if len(cred.BundleURL) != 0 {
		cfg = s.createStorageGatewayConfig(cred)
	} else {
		cfg = s.createDirectS3OpaConfig(*cred.ObjectStore)
	}
	cfg.Plugins = map[string]bool{"dcl": true}
	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	bCfg, _ := json.Marshal(cfg)
	s.Log.Debug("OPA Config: '%s'", string(bCfg))
	return libbuildpack.NewJSON().Write(filePath, cfg)
}

func (s *Supplier) createDirectS3OpaConfig(osCreds services.ObjectStoreCredentials) OPAConfig {
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

func (s *Supplier) createStorageGatewayConfig(cred services.AMSCredentials) OPAConfig {
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
			Cert: cred.CertPath,
			Key:  cred.KeyPath,
		}},
	}

	return OPAConfig{
		Bundles:  bundles,
		Services: services,
	}
}

func (s *Supplier) writeLaunchConfig(cfg env.Config) error {
	s.Log.Info("writing launch.yml..")
	cmd := fmt.Sprintf(
		`"/home/vcap/deps/%s" run -s -c "/home/vcap/deps/%s" -l '%s' -a '127.0.0.1:%d' --skip-version-check`,
		path.Join(s.Stager.DepsIdx(), "opa"),
		path.Join(s.Stager.DepsIdx(), "opa_config.yml"),
		cfg.LogLevel,
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

func (s *Supplier) upload(cfg env.Config, amsCreds services.AMSCredentials) error {
	// s.Log.Info("creating policy archive..")
	// buf, err := uploader.CreateArchive(s.Log, cfg.root)
	// if err != nil {
	// 	return fmt.Errorf("could not create policy DCL.tar.gz: %w", err)
	// }
	uploader, err := s.UploadBuilder(s.Log, amsCreds.CertPath, amsCreds.KeyPath)
	if err != nil {
		return fmt.Errorf("unable to create uploader: %s", err)
	}
	return uploader.Upload(path.Join(s.Stager.BuildDir(), cfg.Root), amsCreds.URL)
}
