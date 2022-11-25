package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/SAP/cloud-authorization-buildpack/pkg/common/services"
)

var log = &Logger{}

func main() {
	// 0=program name; 1=dep-dir
	if len(os.Args) < 1 {
		log.Error("Cert-To-Disk: Missing dependency directory argument\n\nUsage: %s <dependency-directory>", os.Args[0])
		os.Exit(1)
	}
	amsStagerDepDir := os.Args[1]

	cert, key, err := loadCert()
	if err != nil {
		if errors.Is(err, ErrMegacliteMode) {
			log.Info("AMS sidecar starting in megaclite mode, using CF instance certificate")
			os.Exit(0)
		}
		log.Error("Error starting AMS sidecar: %v", err)
		os.Exit(1)
	}

	err = copyCertToDisk(amsStagerDepDir, cert, key)
	if err != nil {
		log.Error("Error starting AMS sidecar: %v", err)
		os.Exit(1)
	}

	log.Info("Successfully copied ias cert to folder '%s' on disk, terminating cert-to-disk helper. This will result in an Exit status 0 in the app logs. The main AMS sidecar is not effected", amsStagerDepDir)
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

var ErrMegacliteMode = errors.New("AMS sidecar starting in megaclite mode: No cert-to-disk required")

func loadCert() (cert, key []byte, err error) {
	identityCreds, err := services.LoadServiceCredentials(log)
	if err != nil {
		return nil, nil, fmt.Errorf("could not load AMSCredentials: %w", err)
	}

	if identityCreds.AmsInstanceID == services.MegacliteID {
		return nil, nil, ErrMegacliteMode
	}

	return []byte(identityCreds.Certificate), []byte(identityCreds.Key), nil
}

var _ services.Logger = &Logger{}

type Logger struct{}

func (l *Logger) Info(format string, args ...interface{}) { //nolint:goprintffuncname
	_, _ = fmt.Fprintln(os.Stdout, fmt.Sprintf(format, args...))
}

func (l *Logger) Error(format string, args ...interface{}) { //nolint:goprintffuncname
	_, _ = fmt.Fprintln(os.Stderr, fmt.Sprintf(format, args...))
}
