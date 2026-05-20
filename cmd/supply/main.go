package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"

	"github.com/cloudfoundry/libbuildpack"
)

func main() {
	logger := libbuildpack.NewLogger(os.Stdout)

	// Print the deprecation banner as the very first thing so it is impossible
	// to miss in the CF staging logs.
	supply.PrintDeprecation(logger)

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
	version, err := manifest.Version()
	if err != nil {
		logger.Error("Unable to load buildpack version: %s", err)
		os.Exit(20)
	}

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

	if err = manifest.ApplyOverride(stager.DepsDir()); err != nil {
		logger.Error("Unable to apply override.yml files: %s", err)
		os.Exit(17)
	}

	err = libbuildpack.RunBeforeCompile(stager)
	if err != nil {
		logger.Error("Before Compile: %s", err)
		os.Exit(12)
	}

	err = stager.SetStagingEnvironment()
	if err != nil {
		logger.Error("Unable to setup environment variables: %s", err)
		os.Exit(14)
	}
	s := supply.Supplier{
		Stager:           stager,
		Log:              logger,
		BuildpackDir:     buildpackDir,
		GetClient:        uploader.GetClient,
		BuildpackVersion: version,
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

	// Re-print the deprecation banner at the very end of supply, so it is
	// also the last thing visible in the staging logs.
	supply.PrintDeprecation(logger)
}
