package supply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/open-policy-agent/opa/download"
	"github.com/open-policy-agent/opa/plugins/bundle"

	"github.com/SAP/cloud-authorization-buildpack/pkg/common/services"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/env"
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
	Manifest            Manifest
	Installer           Installer
	Stager              *libbuildpack.Stager
	Command             Command
	Log                 *libbuildpack.Logger
	BuildpackDir        string
	GetClient           func(cert, key []byte) (uploader.AMSClient, error)
	CertCopierSourceDir string
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
	amsCreds, err := services.LoadAMSCredentials(s.Log)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	tlsCfg, err := s.getTLSConfig(&amsCreds)
	if err != nil {
		return fmt.Errorf("could not load TLS credentials: %w", err)
	}
	if err := s.supplyOPABinary(); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.supplyCertCopier(); err != nil {
		return fmt.Errorf("could not supply cert-copier binary: %w", err)
	}
	if err := s.writeLaunchConfig(cfg); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(amsCreds, tlsCfg); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeProfileDFile(cfg); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}
	if cfg.ShouldUpload {
		if err := s.upload(amsCreds, tlsCfg, cfg.Root); err != nil {
			return fmt.Errorf("error uploading policies: %w", err)
		}
	}
	return nil
}

type tlsConfig struct {
	CertPath string
	KeyPath  string
	Cert     []byte
	Key      []byte
}

func (s *Supplier) getTLSConfig(amsCreds *services.AMSCredentials) (tlsConfig, error) {
	if amsCreds.InstanceID == services.MegacliteID {
		cert, err := os.ReadFile(os.Getenv("CF_INSTANCE_CERT"))
		if err != nil {
			return tlsConfig{}, fmt.Errorf("unable to read CF_INSTANCE_CERT certificate: %s", err)
		}
		key, err := os.ReadFile(os.Getenv("CF_INSTANCE_KEY"))
		if err != nil {
			return tlsConfig{}, fmt.Errorf("unable to read CF_INSTANCE_KEY certificate: %s", err)
		}
		return tlsConfig{
			CertPath: "${CF_INSTANCE_CERT}",
			KeyPath:  "${CF_INSTANCE_KEY}",
			Cert:     cert,
			Key:      key,
		}, nil
	}
	cert, key, err := services.LoadIASClientCert(s.Log)
	if err != nil {
		return tlsConfig{}, fmt.Errorf("unable to load identity client certificate: %s", err)
	}
	// The identity cert is written to the deps directory during app startup by the separate app vcap-cert-copier.go
	return tlsConfig{
		CertPath: path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.crt"),
		KeyPath:  path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.key"),
		Cert:     cert,
		Key:      key,
	}, err
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
	URL         string `json:"url"`
	Headers     map[string]string
	Credentials Credentials `json:"credentials"`
}

type OPAConfig struct {
	Bundles  map[string]*bundle.Source `json:"bundles"`
	Services map[string]RestConfig     `json:"services"`
	Plugins  map[string]bool           `json:"plugins,omitempty"`
}

func (s *Supplier) writeProfileDFile(cfg env.Config) error {
	s.Log.Info("writing profileD file..")
	values := map[string]string{
		"OPA_URL": fmt.Sprintf("http://localhost:%d/", cfg.Port),
		"ADC_URL": fmt.Sprintf("http://localhost:%d/", cfg.Port),
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
	return os.WriteFile(path.Join(s.Stager.ProfileDir(), "0000_opa_env.sh"), b.Bytes(), 0755) //nolint
}

func (s *Supplier) writeOpaConfig(cred services.AMSCredentials, tlsCfg tlsConfig) error {
	s.Log.Info("writing opa config..")

	var cfg OPAConfig
	if cred.BundleURL == "" {
		cfg = s.createDirectS3OpaConfig(*cred.ObjectStore)
	} else {
		cfg = s.createStorageGatewayConfig(cred, tlsCfg)
	}
	cfg.Plugins = map[string]bool{"dcl": true}
	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	bCfg, _ := json.Marshal(cfg)
	s.Log.Debug("OPA config: '%s'", string(bCfg))
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
	svcs := make(map[string]RestConfig)
	svcs[serviceKey] = RestConfig{
		URL:         fmt.Sprintf("https://%s/%s", osCreds.Host, osCreds.Bucket),
		Credentials: Credentials{S3Signing: &S3Signing{AWSEnvCreds: struct{}{}}},
	}

	return OPAConfig{
		Bundles:  bundles,
		Services: svcs,
	}
}

func (s *Supplier) createStorageGatewayConfig(cred services.AMSCredentials, cfg tlsConfig) OPAConfig {
	serviceKey := "bundle_storage"
	bundles := make(map[string]*bundle.Source)
	bundles[cred.InstanceID] = &bundle.Source{
		Config: download.Config{
			Polling: download.PollingConfig{
				MinDelaySeconds: newInt64P(10),
				MaxDelaySeconds: newInt64P(20),
			},
		},
		Service: serviceKey,

		Resource: cred.InstanceID + ".tar.gz",
	}
	svcs := make(map[string]RestConfig)
	svcs[serviceKey] = RestConfig{
		URL:     cred.BundleURL,
		Headers: map[string]string{env.HeaderInstanceID: cred.InstanceID},
		Credentials: Credentials{ClientTLS: &ClientTLS{
			Cert: cfg.CertPath,
			Key:  cfg.KeyPath,
		}},
	}

	return OPAConfig{
		Bundles:  bundles,
		Services: svcs,
	}
}

func (s *Supplier) writeLaunchConfig(cfg env.Config) error {
	s.Log.Info("writing launch.yml..")
	cmd := fmt.Sprintf(
		`%q && %q run -s -c %q -l '%s' -a '127.0.0.1:%d' --skip-version-check`,
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "bin", "cert-copier"),
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "opa"),
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "opa_config.yml"),
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

func (s *Supplier) supplyCertCopier() error {
	sourceFile := path.Join(s.CertCopierSourceDir, "cert-copier")
	destFile := path.Join(s.Stager.DepDir(), "bin", "cert-copier")
	err := libbuildpack.CopyFile(sourceFile, destFile)
	if err != nil {
		return fmt.Errorf("couldn't copy cert-copier dependency: %w", err)
	}

	// The packager overwrites the permissions, so we need to make it executable again
	return os.Chmod(destFile, 0755)
}

func (s *Supplier) upload(amsCreds services.AMSCredentials, tlsCfg tlsConfig, rootDir string) error {
	client, err := s.GetClient(tlsCfg.Cert, tlsCfg.Key)
	if err != nil {
		return fmt.Errorf("unable to create AMS client: %s", err)
	}
	u := uploader.Uploader{
		Log:           s.Log,
		Root:          path.Join(s.Stager.BuildDir(), rootDir),
		Client:        client,
		AMSInstanceID: amsCreds.InstanceID,
	}
	return u.Do(context.Background(), amsCreds.URL)
}
