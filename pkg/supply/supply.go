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
	identityCreds, err := services.LoadServiceCredentials(s.Log)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	tlsCfg, err := s.getTLSConfig(identityCreds)
	if err != nil {
		return fmt.Errorf("could not load TLS credentials: %w", err)
	}
	if err := s.supplyOPABinary(); err != nil {
		return fmt.Errorf("could not supply opa binary: %w", err)
	}
	if err := s.supplyCertCopier(); err != nil {
		return fmt.Errorf("could not supply cert-to-disk binary: %w", err)
	}
	if err := s.writeLaunchConfig(cfg); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(identityCreds, tlsCfg); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeProfileDFile(cfg); err != nil {
		return fmt.Errorf("could not write profileD file: %w", err)
	}
	if cfg.ShouldUpload {
		if err := s.upload(identityCreds, tlsCfg, cfg.Root); err != nil {
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

func (s *Supplier) getTLSConfig(identityCreds *services.IASCredentials) (tlsConfig, error) {
	if identityCreds.AmsInstanceID == services.MegacliteID {
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

	// The identity cert is written to the deps directory during app startup by the separate app cert-to-disk.go
	return tlsConfig{
		CertPath: path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.crt"),
		KeyPath:  path.Join("/home/vcap/deps/", s.Stager.DepsIdx(), "ias.key"),
		Cert:     []byte(identityCreds.Certificate),
		Key:      []byte(identityCreds.Key),
	}, nil
}

type ClientTLS struct {
	Cert string `json:"cert,omitempty"`
	Key  string `json:"private_key,omitempty"`
}

type Credentials struct {
	ClientTLS *ClientTLS `json:"client_tls,omitempty"` // storage gateway bundle access
}

type OPARestConfig struct {
	URL         string `json:"url"`
	Headers     map[string]string
	Credentials Credentials `json:"credentials"`
}

type OPAConfig struct {
	Bundles  map[string]*bundle.Source `json:"bundles"`
	Services map[string]OPARestConfig  `json:"services"`
	Plugins  map[string]bool           `json:"plugins,omitempty"`
	Status   map[string]string         `json:"status,omitempty"`
}

func (s *Supplier) writeProfileDFile(cfg env.Config) error {
	s.Log.Info("writing profileD file..")
	values := map[string]string{
		"OPA_URL": fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port),
		"ADC_URL": fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port),
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

func (s *Supplier) writeOpaConfig(creds *services.IASCredentials, tlsCfg tlsConfig) error {
	s.Log.Info("writing opa config..")

	cfg := s.createBundleGatewayConfig(creds, tlsCfg)
	cfg.Plugins = map[string]bool{"dcl": true}
	cfg.Status = map[string]string{"plugin": "dcl"}
	filePath := path.Join(s.Stager.DepDir(), "opa_config.yml")
	bCfg, _ := json.Marshal(cfg)
	s.Log.Debug("OPA config: '%s'", string(bCfg))
	return libbuildpack.NewJSON().Write(filePath, cfg)
}

func (s *Supplier) createBundleGatewayConfig(cred *services.IASCredentials, cfg tlsConfig) OPAConfig {
	serviceKey := "bundle_storage"
	bundles := make(map[string]*bundle.Source)
	bundles[cred.AmsInstanceID] = &bundle.Source{
		Config: download.Config{
			Polling: download.PollingConfig{
				MinDelaySeconds: newInt64P(10),
				MaxDelaySeconds: newInt64P(20),
			},
		},
		Service: serviceKey,

		Resource: cred.AmsInstanceID + ".tar.gz",
	}
	svcs := make(map[string]OPARestConfig)
	svcs[serviceKey] = OPARestConfig{
		URL:     cred.AmsBundleGatewayURL,
		Headers: map[string]string{env.HeaderInstanceID: cred.AmsInstanceID},
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
		`%q %q && %q run -s -c %q -l '%s' -a '127.0.0.1:%d' --disable-telemetry`,
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "bin", "cert-to-disk"),
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx()),
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "opa"),
		path.Join("/home", "vcap", "deps", s.Stager.DepsIdx(), "opa_config.yml"),
		cfg.LogLevel,
		9888)
	s.Log.Info("OPA start command: '%s'", cmd)
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
	sourceFile := path.Join(s.CertCopierSourceDir, "cert-to-disk")
	destFile := path.Join(s.Stager.DepDir(), "bin", "cert-to-disk")
	err := libbuildpack.CopyFile(sourceFile, destFile)
	if err != nil {
		return fmt.Errorf("couldn't copy cert-to-disk dependency: %w", err)
	}

	// The packager overwrites the permissions, so we need to make it executable again
	return os.Chmod(destFile, 0755)
}

func (s *Supplier) upload(creds *services.IASCredentials, tlsCfg tlsConfig, rootDir string) error {
	client, err := s.GetClient(tlsCfg.Cert, tlsCfg.Key)
	if err != nil {
		return fmt.Errorf("unable to create AMS client: %s", err)
	}
	u := uploader.Uploader{
		Log:           s.Log,
		Root:          path.Join(s.Stager.BuildDir(), rootDir),
		Client:        client,
		AMSInstanceID: creds.AmsInstanceID,
	}
	return u.Do(context.Background(), creds.AmsServerURL)
}
