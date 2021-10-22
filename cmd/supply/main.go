package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"

	"github.com/cloudfoundry/libbuildpack"
)

func main() {
	logger := libbuildpack.NewLogger(os.Stdout)

	buildpackDir, err := libbuildpack.GetBuildpackDir()
	if err != nil {
		logger.Error("Unable to determine buildpack directory: %s", err)
		os.Exit(9)
	}

	manifest, err := libbuildpack.NewManifest(buildpackDir, logger, time.Now())
	if err != nil {
		logger.Error("Unable to load buildpack manifest: %s", err)
		os.Exit(10)
	}
	installer := libbuildpack.NewInstaller(manifest)

	stager := libbuildpack.NewStager(os.Args[1:], logger, manifest)
	if err := stager.CheckBuildpackValid(); err != nil {
		os.Exit(11)
	}

	if err := os.MkdirAll(filepath.Join(stager.DepDir(), "bin"), 0755); err != nil {
		os.Exit(11)
	}
	if err := os.MkdirAll(filepath.Join(stager.DepDir(), "lib"), 0755); err != nil {
		os.Exit(11)
	}

	if err = installer.SetAppCacheDir(stager.CacheDir()); err != nil {
		logger.Error("Unable to setup app cache dir: %s", err)
		os.Exit(18)
	}
	if err = manifest.ApplyOverride(stager.DepsDir()); err != nil {
		logger.Error("Unable to apply override.yml files: %s", err)
		os.Exit(17)
	}

	err = libbuildpack.RunBeforeCompile(stager)
	if err != nil {
		logger.Error("Before Compile: %s", err)
		os.Exit(12)
	}

	if err := os.MkdirAll(filepath.Join(stager.DepDir(), "bin"), 0755); err != nil {
		logger.Error("Unable to create bin directory: %s", err)
		os.Exit(13)
	}

	err = stager.SetStagingEnvironment()
	if err != nil {
		logger.Error("Unable to setup environment variables: %s", err)
		os.Exit(14)
	}
	cert, key, err := loadIASClientCert(logger)
	if err != nil {
		logger.Error("Unable to laod IAS client certificate: %s", err)
		os.Exit(14)
	}
	os.WriteFile(filepath.Join(stager.DepDir(), "ias.crt"), cert, 0666)
	if err != nil {
		logger.Error("Unable to write IAS client certificate: %s", err)
		os.Exit(14)
	}
	os.WriteFile(filepath.Join(stager.DepDir(), "ias.key"), key, 0666)
	if err != nil {
		logger.Error("Unable to write IAS client key: %s", err)
		os.Exit(14)
	}

	uploader, err := uploader.NewUploader(logger, cert, key)
	if err != nil {
		logger.Error("Unable to create uploader: %s", err)
		os.Exit(15)
	}
	s := supply.Supplier{
		Manifest:     manifest,
		Installer:    installer,
		Stager:       stager,
		Command:      &libbuildpack.Command{},
		Log:          logger,
		BuildpackDir: buildpackDir,
		Uploader:     uploader,
	}

	err = s.Run()
	if err != nil {
		logger.Error("Error: %s", err)
		os.Exit(15)
	}

	if err := stager.WriteConfigYml(nil); err != nil {
		logger.Error("Error writing config.yml: %s", err)
		os.Exit(16)
	}
	if err = installer.CleanupAppCache(); err != nil {
		logger.Error("Unable clean up app cache: %s", err)
		os.Exit(19)
	}
}
func loadIASClientCert(log *libbuildpack.Logger) (cert []byte, key []byte, err error) {
	iasCredsRaw, _, err := supply.LoadServiceCredentials(log, "identity")
	if err != nil {
		return cert, key, err
	}
	var iasCreds supply.IASCredentials
	err = json.Unmarshal(iasCredsRaw, &iasCreds)
	if err != nil {
		return cert, key, err
	}
	if iasCreds.Certificate == "" {
		return cert, key, fmt.Errorf("identity service binding does not contain client certificate. Please use binding parameter {\"credential_type\":\"X509_GENERATED\"}")
	}

	return []byte(iasCreds.Certificate), []byte(iasCreds.Key), nil
}
