package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v2"

	"github.com/SAP/cloud-authorization-buildpack/pkg/common/services"
)

const amsBuildpackName = "opa"

func main() {
	log := &Logger{}

	amsCreds, err := services.LoadAMSCredentials(log)
	if err != nil {
		log.Error("Error starting AMS sidecar: could not load AMSCredentials: %v", err)
		os.Exit(1)
	}

	if amsCreds.InstanceID == services.MegacliteID {
		log.Info("AMS sidecar starting in megaclite mode, using CF instance certificate")
		os.Exit(0)
	}

	depsRoot := path.Join("/home", "vcap", "deps")
	amsStagerDepDir, err := getAMSDependencyDir(depsRoot)
	if err != nil {
		log.Error("Error starting AMS sidecar: %v", err)
		os.Exit(1)
	}

	cert, key, err := services.LoadIASClientCert(log)
	if err != nil {
		log.Error("Error starting AMS sidecar: unable to load identity client certificate: %v", err)
		os.Exit(1)
	}

	err = copyCertToDisk(amsStagerDepDir, cert, key)
	if err != nil {
		log.Error("Error starting AMS sidecar: %v", err)
		os.Exit(1)
	}

	log.Info("Successfully copied ias cert to disk, terminating cert-copy helper. This will result in an Exit status 0 in the app logs. The main AMS sidecar is not effected")
}

type DependencyConfig struct {
	Name string `yaml:"name"`
}

func getAMSDependencyDir(depsRoot string) (string, error) {
	depDirs, err := os.ReadDir(depsRoot)
	if err != nil {
		return "", fmt.Errorf("error listing dependency directories: %w", err)
	}

	var intermediateErrs []error
	for i, dir := range depDirs {
		if !dir.IsDir() {
			continue
		}

		currentAbsoluteDir := path.Join(depsRoot, dir.Name())
		configFile, err := os.ReadFile(path.Join(currentAbsoluteDir, "config.yml"))
		if err != nil {
			intermediateErrs = append(intermediateErrs, fmt.Errorf("error reading config file of dependency %d: %w", i, err))
		}
		var config DependencyConfig
		err = yaml.Unmarshal(configFile, &config)
		if err != nil {
			intermediateErrs = append(intermediateErrs, fmt.Errorf("error unmarshalling config file of dependency %d: %v", i, err))
		}

		if config.Name == amsBuildpackName {
			return currentAbsoluteDir, nil
		}
	}

	if len(intermediateErrs) > 0 {
		return "", fmt.Errorf("could not find ams dependency directory, following intermediate errors appeared: %v", intermediateErrs)
	}
	return "", fmt.Errorf("could not find ams dependency directory")
}

func copyCertToDisk(amsDependencyDir string, cert, key []byte) error {
	err := os.WriteFile(path.Join(amsDependencyDir, "ias.crt"), cert, 0600)
	if err != nil {
		return fmt.Errorf("unable to write IAS client certificate: %s", err)
	}
	err = os.WriteFile(filepath.Join(amsDependencyDir, "ias.key"), key, 0600)
	if err != nil {
		return fmt.Errorf("unable to write IAS client certificate key: %s", err)
	}

	return nil
}

var _ services.Logger = &Logger{}

type Logger struct{}

func (l *Logger) Info(format string, args ...interface{}) { //nolint:goprintffuncname
	_, _ = fmt.Fprintln(os.Stdout, fmt.Sprintf(format, args...))
}

func (l *Logger) Error(format string, args ...interface{}) { //nolint:goprintffuncname
	_, _ = fmt.Fprintln(os.Stderr, fmt.Sprintf(format, args...))
}
