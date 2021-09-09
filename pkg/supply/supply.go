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
	Manifest  Manifest
	Installer Installer
	Stager    Stager
	Command   Command
	Log       *libbuildpack.Logger
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
	s.Log.BeginStep("Supplying pkg")
	if err := s.writeLaunchConfig(); err != nil {
		return fmt.Errorf("could not write launch config: %w", err)
	}
	if err := s.writeOpaConfig(); err != nil {
		return fmt.Errorf("could not write opa config: %w", err)
	}
	if err := s.writeEnvFile(); err != nil {
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

func (s *Supplier) writeEnvFile() error {
	s.Log.Info("writing env file..")
	var b bytes.Buffer
	b.WriteString("export OPA_URL=http://localhost:9888")

	dirPath := path.Join(s.Stager.BuildDir(), ".profile.d")
	err := os.Mkdir(dirPath, 0755)
	if err != nil {
		return fmt.Errorf("could not create profile directory '%s': %w", dirPath, err)
	}

	filePath := path.Join(dirPath, "0000_opa_env.sh")
	err = os.WriteFile(filePath, b.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("could not write file to '%s': %w", filePath, err)
	}
	return nil
}

func (s *Supplier) writeOpaConfig() error {
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
		URL:         "",
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
