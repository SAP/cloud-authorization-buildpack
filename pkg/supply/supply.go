package supply

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/cloudfoundry/libbuildpack"

	"github.com/SAP/cloud-authorization-buildpack/pkg/common/services"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/env"
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
)

// DeprecationBanner is the user-facing notice that this buildpack is deprecated.
// Printed at staging time via libbuildpack.Logger.Warning and at every app start
// via a profile.d script written by writeDeprecationProfileD.
const DeprecationBanner = `================================================================================
  cloud-authorization-buildpack is DEPRECATED

  The OPA sidecar has been removed. This buildpack is now in maintenance-only
  mode and only performs base DCL policy upload. The Authorization Management
  Service will discontinue serving .rego policy bundles soon.

  Please migrate now:
    * Base policy upload  -> use the dedicated policies-deployer task:
        https://sap.github.io/cloud-identity-developer-guide/Authorization/DeployDCL.html
    * Authorization decisions -> use the latest in-memory client libraries
      (no sidecar required):
        https://sap.github.io/cloud-identity-developer-guide/Authorization/GettingStarted.html#dependency-setup
    * Support: https://sap.github.io/cloud-identity-developer-guide/Support.html
================================================================================`

// PrintDeprecation logs the deprecation banner as a buildpack warning.
// Called at the start (and end) of supply so it is impossible to miss in
// staging logs.
func PrintDeprecation(log *libbuildpack.Logger) {
	log.Warning("\n%s", DeprecationBanner)
}

type Supplier struct {
	Stager           *libbuildpack.Stager
	Log              *libbuildpack.Logger
	BuildpackDir     string
	GetClient        func(cert, key []byte) (uploader.AMSClient, error)
	BuildpackVersion string
}

func (s *Supplier) Run() error {
	s.Log.BeginStep("Supplying cloud-authorization-buildpack (DEPRECATED, upload-only)")

	cfg, err := env.LoadBuildpackConfig(s.Log)
	if err != nil {
		return fmt.Errorf("could not load buildpack Config: %w", err)
	}

	if err := s.writeDeprecationProfileD(); err != nil {
		return fmt.Errorf("could not write deprecation profile.d script: %w", err)
	}

	if !cfg.ShouldUpload {
		// Nothing else to do: no policy upload requested. The deprecation
		// banner has already been logged at staging time and will be logged
		// again at every app start by the profile.d script.
		return nil
	}

	identityCreds, err := services.LoadServiceCredentials(s.Log)
	if err != nil {
		return fmt.Errorf("could not load AMSCredentials: %w", err)
	}
	cert, key, err := s.loadUploadTLS(identityCreds)
	if err != nil {
		return fmt.Errorf("could not load TLS credentials: %w", err)
	}

	if err := s.upload(identityCreds, cert, key, cfg.Root); err != nil {
		return fmt.Errorf("error uploading policies: %w", err)
	}
	return nil
}

// loadUploadTLS returns the certificate and private key (PEM bytes) used to
// authenticate the policy upload HTTPS request to the AMS server. For the
// megaclite proxy path, the CF instance certificate/key are used; otherwise
// the certificate/key contained in the IAS service binding.
func (s *Supplier) loadUploadTLS(creds *services.IASCredentials) (cert, key []byte, err error) {
	if creds.AmsInstanceID == services.MegacliteID {
		cert, err = os.ReadFile(os.Getenv("CF_INSTANCE_CERT"))
		if err != nil {
			return nil, nil, fmt.Errorf("unable to read CF_INSTANCE_CERT certificate: %s", err)
		}
		key, err = os.ReadFile(os.Getenv("CF_INSTANCE_KEY"))
		if err != nil {
			return nil, nil, fmt.Errorf("unable to read CF_INSTANCE_KEY certificate: %s", err)
		}
		return cert, key, nil
	}
	return []byte(creds.Certificate), []byte(creds.Key), nil
}

// writeDeprecationProfileD writes a profile.d shell script that echoes the
// deprecation banner to stderr at every app start. This is how we keep the
// "with every start" reminder visible now that there is no buildpack-owned
// runtime process anymore.
func (s *Supplier) writeDeprecationProfileD() error {
	s.Log.Info("writing deprecation profile.d script..")

	script := "#!/bin/bash\n" +
		"cat >&2 <<'__AMS_BUILDPACK_DEPRECATION__'\n" +
		"**WARNING** cloud-authorization-buildpack is DEPRECATED.\n" +
		DeprecationBanner + "\n" +
		"__AMS_BUILDPACK_DEPRECATION__\n"

	// We do not use libbuildpack.WriteProfileD, because the copy mechanism from
	// deps_dir to build_dir does not work for sidecar-style supply buildpacks
	// (deps_dir/profileD).
	if err := os.MkdirAll(s.Stager.ProfileDir(), 0755); err != nil {
		return fmt.Errorf("couldn't create profile dir: %w", err)
	}
	return os.WriteFile(path.Join(s.Stager.ProfileDir(), "0000_ams_deprecation.sh"), []byte(script), 0755) //nolint:gosec
}

func (s *Supplier) upload(creds *services.IASCredentials, cert, key []byte, rootDir string) error {
	client, err := s.GetClient(cert, key)
	if err != nil {
		return fmt.Errorf("unable to create AMS client: %s", err)
	}
	vcapApp := env.LoadVcapApplication(s.Log)

	u := uploader.Uploader{
		Log:           s.Log,
		Root:          path.Join(s.Stager.BuildDir(), rootDir),
		Client:        client,
		AMSInstanceID: creds.AmsInstanceID,
		ExtraHeaders: map[string]string{
			"User-Agent": fmt.Sprintf("cloud-authorization-buildpack/%s", s.BuildpackVersion),
			"X-Appname":  vcapApp.ApplicationName,
		},
	}
	return u.Do(context.Background(), creds.AmsServerURL)
}
