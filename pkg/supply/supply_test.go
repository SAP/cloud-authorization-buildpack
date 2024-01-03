package supply_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/buildpackapplifecycle/buildpackrunner/resources"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo" //nolint
	. "github.com/onsi/gomega" //nolint
	"github.com/open-policy-agent/opa/config"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/open-policy-agent/opa/plugins/rest"
	"gopkg.in/yaml.v2"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/env"
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
	"github.com/SAP/cloud-authorization-buildpack/resources/testdata"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
)

var _ = Describe("Supply", func() {
	var (
		uploadReqSpy    *http.Request
		certSpy, keySpy []byte
		err             error
		buildDir        string
		depsDir         string
		depsIdx         string
		depDir          string
		certCopierDir   string
		supplier        *supply.Supplier
		logger          *libbuildpack.Logger
		mockCtrl        *gomock.Controller
		mockAMSClient   *MockAMSClient
		writtenLogs     *bytes.Buffer
		vcapServices    string
	)

	BeforeEach(func() {
		depsDir, err = os.MkdirTemp("", "test")
		Expect(err).To(BeNil())
		buildDir, err = os.MkdirTemp("", "buildDir")
		Expect(err).To(BeNil())
		certCopierDir, err = os.MkdirTemp("", "certCopierDir")
		Expect(err).To(BeNil())
		err := os.WriteFile(path.Join(certCopierDir, "cert-to-disk"), []byte("dummy file"), 0755) //nolint
		Expect(err).To(BeNil())
		Expect(os.MkdirAll(path.Join(buildDir, "policies"), os.ModePerm)).To(Succeed())
		Expect(libbuildpack.CopyDirectory(path.Join("testdata", "policies"), path.Join(buildDir, "policies"))).To(Succeed())

		depsIdx = "42"
		depDir = filepath.Join(depsDir, depsIdx)

		err = os.MkdirAll(depDir, 0755)
		Expect(err).To(BeNil())

		writtenLogs = new(bytes.Buffer)
		logger = libbuildpack.NewLogger(writtenLogs)

		mockCtrl = gomock.NewController(GinkgoT())
		mockAMSClient = NewMockAMSClient(mockCtrl)
		mockAMSClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			uploadReqSpy = req
			return &http.Response{StatusCode: 200, Body: io.NopCloser(nil)}, nil
		}).AnyTimes()
	})

	JustBeforeEach(func() {
		Expect(os.Setenv("VCAP_SERVICES", vcapServices)).To(Succeed())
		Expect(os.Setenv("CF_STACK", "cflinuxfs3")).To(Succeed())
		wd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		buildpackDir := path.Join(filepath.Dir(filepath.Dir(wd)))

		args := []string{buildDir, "", depsDir, depsIdx}
		bps := libbuildpack.NewStager(args, logger, &libbuildpack.Manifest{})
		m, err := libbuildpack.NewManifest(buildpackDir, logger, time.Now())
		Expect(err).NotTo(HaveOccurred())
		supplier = &supply.Supplier{
			Stager:              bps,
			Manifest:            m,
			Installer:           libbuildpack.NewInstaller(m),
			Log:                 logger,
			BuildpackDir:        buildpackDir,
			CertCopierSourceDir: certCopierDir,
			GetClient: func(cert, key []byte) (uploader.AMSClient, error) {
				certSpy = cert
				keySpy = key
				return mockAMSClient, nil
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()

		err = os.RemoveAll(depsDir)
		Expect(err).To(BeNil())
		Expect(os.Unsetenv("VCAP_APPLICATION")).To(Succeed())
		Expect(os.Unsetenv("AMS_DCL_ROOT")).To(Succeed())
		Expect(os.Unsetenv("AMS_SERVICE")).To(Succeed())
		Expect(os.Unsetenv("CF_STACK")).To(Succeed())
		Expect(os.Unsetenv("VCAP_SERVICES")).To(Succeed())
	})
	When("AMS credentials are included in the IAS credentials", func() {
		Context("and credential type is not x509", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithIASAuthWithClientSecret
				os.Setenv("AMS_DCL_ROOT", "/policies")
			})
			It("should fail", func() {
				err := supplier.Run()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("and VCAP_SERVICES contains user-provided 'megaclite' service instance from DwC", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithMegacliteAndIAS
				os.Setenv("AMS_DCL_ROOT", "/policies")
			})
			It("should configure access to the gateway directly", func() {
				Expect(supplier.Run()).To(Succeed())
				rawConfig, err := os.ReadFile(filepath.Join(depDir, "opa_config.yml"))
				Expect(err).NotTo(HaveOccurred())
				cfg, err := config.ParseConfig(rawConfig, "testId")
				Expect(err).NotTo(HaveOccurred())

				var restConfig map[string]rest.Config
				err = json.Unmarshal(cfg.Services, &restConfig)
				Expect(err).NotTo(HaveOccurred())
				By("specifying ClientTLS", func() {
					Expect(restConfig).To(HaveKey("bundle_storage"))
					Expect(restConfig["bundle_storage"].Credentials.ClientTLS.Cert).To(Equal("/home/vcap/deps/42/ias.crt"))
					Expect(restConfig["bundle_storage"].Credentials.ClientTLS.PrivateKey).To(Equal("/home/vcap/deps/42/ias.key"))
				})
				By("extending the tenant host URL from the identity service", func() {
					Expect(restConfig["bundle_storage"].URL).NotTo(ContainSubstring("megaclite.host"))
					Expect(restConfig["bundle_storage"].URL).To(Equal("https://mytenant.accounts400.ondemand.com/bundle-gateway"))
				})
			})
		})
		Context("and credential type is x509", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithIASAuthX509
				os.Setenv("AMS_DCL_ROOT", "/policies")
			})
			It("should succeed", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(filepath.Join(depDir, "launch.yml")).To(BeARegularFile())
				Expect(string(keySpy)).To(Equal("identity-key-payload"))
				Expect(string(certSpy)).To(Equal("identity-cert-payload"))
			})
			It("sets the ams instance id http header when uploading the bundle", func() {
				Expect(supplier.Run()).To(Succeed())
				expectedValue := []string{"00000000-3b4d-4c41-9e5b-9aee7bfa6348"}
				Expect(uploadReqSpy.Header).Should(HaveKeyWithValue(env.HeaderInstanceID, expectedValue))
			})
			It("creates a valid launch.yml", func() {
				Expect(supplier.Run()).To(Succeed())
				launchConfig, err := os.Open(filepath.Join(depDir, "launch.yml"))
				Expect(err).NotTo(HaveOccurred())
				var ld resources.LaunchData
				err = yaml.NewDecoder(launchConfig).Decode(&ld)
				Expect(err).NotTo(HaveOccurred())

				By("specifying proper options", func() {
					Expect(ld.Processes).To(HaveLen(1))
					Expect(ld.Processes[0].Type).To(Equal("opa"))
					Expect(ld.Processes[0].Platforms.Cloudfoundry.SidecarFor).To(Equal([]string{"web"}))
					cmd := `"/home/vcap/deps/42/bin/cert-to-disk" "/home/vcap/deps/42" && "/home/vcap/deps/42/opa" run -s -c "/home/vcap/deps/42/opa_config.yml" -l 'error' -a '127.0.0.1:9888' --disable-telemetry`
					Expect(ld.Processes[0].Command).To(Equal(cmd))
					Expect(ld.Processes[0].Limits.Memory).To(Equal(100))
					Expect(writtenLogs.String()).To(ContainSubstring("writing launch.yml"))
				})
			})
			It("creates the correct opa config", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(writtenLogs.String()).To(ContainSubstring("writing opa config"))

				rawConfig, err := os.ReadFile(filepath.Join(depDir, "opa_config.yml"))
				Expect(err).NotTo(HaveOccurred())
				cfg, err := config.ParseConfig(rawConfig, "testId")
				Expect(err).NotTo(HaveOccurred())

				var serviceKey string
				By("specifying the correct bundle options", func() {
					var bundleConfig map[string]bundle.Source
					err = json.Unmarshal(cfg.Bundles, &bundleConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(bundleConfig).To(HaveKey("00000000-3b4d-4c41-9e5b-9aee7bfa6348"))
					serviceKey = bundleConfig["00000000-3b4d-4c41-9e5b-9aee7bfa6348"].Service
					Expect(serviceKey).NotTo(BeEmpty())
					Expect(bundleConfig["00000000-3b4d-4c41-9e5b-9aee7bfa6348"].Resource).To(Equal("00000000-3b4d-4c41-9e5b-9aee7bfa6348.tar.gz"))
					Expect(*bundleConfig["00000000-3b4d-4c41-9e5b-9aee7bfa6348"].Polling.MinDelaySeconds).To(Equal(int64(10)))
					Expect(*bundleConfig["00000000-3b4d-4c41-9e5b-9aee7bfa6348"].Polling.MaxDelaySeconds).To(Equal(int64(20)))
				})
				By("enabling the OPA dcn plugin", func() {
					enabled, ok := cfg.Plugins["dcl"]
					Expect(ok).To(BeTrue())
					Expect(string(enabled)).To(Equal(`true`))
				})
				By("enabling the OPA dcl status plugin", func() {
					enabled := cfg.Status
					Expect(string(enabled)).To(Equal(`{"plugin":"dcl"}`))
				})
			})
			It("should configure access to the gateway", func() {
				Expect(supplier.Run()).To(Succeed())
				rawConfig, err := os.ReadFile(filepath.Join(depDir, "opa_config.yml"))
				Expect(err).NotTo(HaveOccurred())
				cfg, err := config.ParseConfig(rawConfig, "testId")
				Expect(err).NotTo(HaveOccurred())

				var restConfig map[string]rest.Config
				err = json.Unmarshal(cfg.Services, &restConfig)
				Expect(err).NotTo(HaveOccurred())
				By("specifying ClientTLS", func() {
					Expect(restConfig).To(HaveKey("bundle_storage"))
					Expect(restConfig["bundle_storage"].Credentials.ClientTLS.Cert).To(Equal("/home/vcap/deps/42/ias.crt"))
					Expect(restConfig["bundle_storage"].Credentials.ClientTLS.PrivateKey).To(Equal("/home/vcap/deps/42/ias.key"))
				})
				By("extending the tenant host URL from the identity service", func() {
					Expect(restConfig["bundle_storage"].URL).To(Equal("https://mytenant.accounts400.ondemand.com/bundle-gateway"))
				})
				By("making sure there's only one auth method", func() {
					Expect(restConfig["bundle_storage"].Credentials.S3Signing).To(BeNil())
				})
				By("enabling the OPA dcn plugin", func() {
					enabled, ok := cfg.Plugins["dcl"]
					Expect(ok).To(BeTrue())
					Expect(string(enabled)).To(Equal(`true`))
				})
				By("uploading to the correct URL", func() {
					Expect(uploadReqSpy.URL.String()).To(Equal("https://mytenant.accounts400.ondemand.com/authorization/sap/ams/v1/ams-instances/00000000-3b4d-4c41-9e5b-9aee7bfa6348/dcl-upload"))
				})
			})
			It("creates the correct env vars", func() {
				Expect(supplier.Run()).To(Succeed())
				env, err := os.ReadFile(path.Join(buildDir, ".profile.d", "0000_opa_env.sh"))
				Expect(err).NotTo(HaveOccurred())
				expectIsExecutable(path.Join(buildDir, ".profile.d", "0000_opa_env.sh"))
				Expect(string(env)).To(ContainSubstring(`export OPA_URL=http://127.0.0.1:9888`))
			})
			It("provides the OPA executable", func() {
				Expect(supplier.Run()).To(Succeed())
				expectIsExecutable(filepath.Join(depDir, "opa"))
			})
			It("provides cert-to-disk executable", func() {
				Expect(supplier.Run()).To(Succeed())
				expectIsExecutable(filepath.Join(depDir, "bin", "cert-to-disk"))
			})
			It("uploads DCL and json files in a bundle", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(uploadReqSpy.Body).NotTo(BeNil())
				files := getTgzFileNames(uploadReqSpy.Body)
				Expect(files).To(ContainElements("myPolicies0/policy0.dcl", "myPolicies1/policy1.dcl", "schema.dcl"))
				Expect(files).NotTo(ContainElements("non-dcl-file.xyz", ContainSubstring("data.json")))
			})
			When("AMS_DCL_ROOT is not set", func() {
				BeforeEach(func() {
					Expect(os.Unsetenv("AMS_DCL_ROOT")).To(Succeed())
					Expect(os.Unsetenv("AMS_SERVICE")).To(Succeed())
				})
				It("creates a warning", func() {
					Expect(supplier.Run()).To(Succeed())
					Expect(writtenLogs.String()).To(ContainSubstring("upload no authorization data"))
				})
			})
			When("AMS_DATA is set", func() {
				BeforeEach(func() {
					os.Setenv("AMS_DATA", "{\"root\":\"/policies\"}")
				})
				It("uploads DCL and json files in a bundle", func() {
					Expect(supplier.Run()).To(Succeed())
					Expect(uploadReqSpy.Body).NotTo(BeNil())
					files := getTgzFileNames(uploadReqSpy.Body)
					Expect(files).To(ContainElements("myPolicies0/policy0.dcl", "myPolicies1/policy1.dcl", "schema.dcl"))
					Expect(files).NotTo(ContainElements("non-dcl-file.xyz", ContainSubstring("data.json")))
				})
				It("creates a warning", func() {
					Expect(supplier.Run()).To(Succeed())
					Expect(writtenLogs.String()).To(ContainSubstring("the environment variable AMS_DATA is deprecated."))
				})
				AfterEach(func() {
					os.Unsetenv("AMS_DATA")
				})
			})
			When("the AMS server returns an error", func() {
				Context("400", func() {
					BeforeEach(func() {
						mockAMSClient = NewMockAMSClient(mockCtrl)
						mockAMSClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
							uploadReqSpy = req
							return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("your policy is broken"))}, nil
						}).AnyTimes()

					})
					It("should log the response body", func() {
						err := supplier.Run()
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("your policy is broken"))
					})
				})
				Context("401 (proof-token endpoint not ready)", func() {
					BeforeEach(func() {
						uploader.RetryPeriod = time.Millisecond * 10
						mockAMSClient = NewMockAMSClient(mockCtrl)
						gomock.InOrder(
							mockAMSClient.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("could not find certificate"))}, nil),
							mockAMSClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
								uploadReqSpy = req
								return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
							}))
					})
					It("retries", func() {
						Expect(supplier.Run()).To(Succeed())
						Expect(writtenLogs.String()).To(ContainSubstring("retrying after"))
						Expect(uploadReqSpy.Body).NotTo(BeNil())
					})
				})
			})
			When("AMS_LOG_LEVEL is set to info", func() {
				BeforeEach(func() { os.Setenv("AMS_LOG_LEVEL", "info") })
				AfterEach(func() { os.Unsetenv("AMS_LOG_LEVEL") })
				It("should start OPA with log level 'info'", func() {
					Expect(supplier.Run()).To(Succeed())
					launchConfig, err := os.Open(filepath.Join(depDir, "launch.yml"))
					Expect(err).NotTo(HaveOccurred())

					var ld resources.LaunchData
					err = yaml.NewDecoder(launchConfig).Decode(&ld)
					Expect(err).NotTo(HaveOccurred())
					Expect(ld.Processes[0].Command).To(ContainSubstring("-l 'info'"))
				})
			})
		})
	})
	When("the bound AMS enabled IAS service is user-provided", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithUserProvidedIAS
			os.Setenv("AMS_DCL_ROOT", "/policies")
		})
		It("should succeed", func() {
			Expect(supplier.Run()).To(Succeed())
		})
	})
	When("VCAP_SERVICES is empty", func() {
		JustBeforeEach(func() {
			os.Unsetenv("VCAP_SERVICES")
		})
		It("should abort with err", func() {
			err := supplier.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not unmarshal VCAP_SERVICES"))
		})
	})
	When("VCAP_SERVICES contains user-provided 'megaclite' service instance from DwC", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithMegaclite
			os.Setenv("AMS_DCL_ROOT", "/policies")
			os.Setenv("CF_INSTANCE_CERT", "testdata/cf_instance_cert.pem")
			os.Setenv("CF_INSTANCE_KEY", "testdata/cf_instance_key.pem")

		})
		AfterEach(func() {
			os.Unsetenv("AMS_DCL_ROOT")
			os.Unsetenv("CF_INSTANCE_CERT")
			os.Unsetenv("CF_INSTANCE_KEY")
		})
		It("should succeed", func() {
			Expect(supplier.Run()).To(Succeed())
			Expect(filepath.Join(depDir, "launch.yml")).To(BeARegularFile())
			Expect(uploadReqSpy.Host).To(Equal("megaclite.host"))
			Expect(string(keySpy)).To(Equal("cf-instance-key-payload"))
			Expect(string(certSpy)).To(Equal("cf-instance-cert-payload"))
		})
		It("should configure OPA to access megaclite", func() {
			Expect(supplier.Run()).To(Succeed())
			rawConfig, err := os.ReadFile(filepath.Join(depDir, "opa_config.yml"))
			Expect(err).NotTo(HaveOccurred())
			cfg, err := config.ParseConfig(rawConfig, "testId")
			Expect(err).NotTo(HaveOccurred())

			var restConfig map[string]rest.Config
			err = json.Unmarshal(cfg.Services, &restConfig)
			Expect(err).NotTo(HaveOccurred())

			var bundlesConfig map[string]*bundle.Source
			err = json.Unmarshal(cfg.Bundles, &bundlesConfig)
			Expect(err).NotTo(HaveOccurred())

			By("specifying ClientTLS", func() {
				Expect(restConfig).To(HaveKey("bundle_storage"))
				Expect(restConfig["bundle_storage"].Credentials.ClientTLS.Cert).To(Equal("${CF_INSTANCE_CERT}"))
				Expect(restConfig["bundle_storage"].Credentials.ClientTLS.PrivateKey).To(Equal("${CF_INSTANCE_KEY}"))
				Expect(restConfig["bundle_storage"].URL).To(Equal("http://megaclite.host/ams/bundle/"))
			})
			By("Using Instance ID placeholder for megaclite", func() {
				Expect(bundlesConfig).To(HaveKey("dwc-megaclite-ams-instance-id"))
				Expect(bundlesConfig["dwc-megaclite-ams-instance-id"].Resource).To(Equal("dwc-megaclite-ams-instance-id.tar.gz"))
			})
			By("making sure there's only one auth method", func() {
				Expect(restConfig["bundle_storage"].Credentials.S3Signing).To(BeNil())
			})
			By("enabling the OPA dcn plugin", func() {
				enabled, ok := cfg.Plugins["dcl"]
				Expect(ok).To(BeTrue())
				Expect(string(enabled)).To(Equal(`true`))
			})
		})
	})
	When("the identity certificate is expired", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithIASAuthX509Expired
			os.Setenv("AMS_DCL_ROOT", "/policies")
		})
		It("fails with proper error message", func() {
			err = supplier.Run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("identity certificate has expired:"))
		})
	})
})

func expectIsExecutable(fp string) {
	fi, err := os.Stat(fp)
	Expect(err).NotTo(HaveOccurred())
	// Check if executable by all
	Expect(fi.Mode().Perm() & 0111).To(Equal(fs.FileMode(0111)))
}

func getTgzFileNames(r io.Reader) []string {
	var files []string
	gzReader, err := gzip.NewReader(r)
	Expect(err).NotTo(HaveOccurred())
	defer gzReader.Close()
	tarGzReader := tar.NewReader(gzReader)
	for {
		header, err := tarGzReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		switch header.Typeflag {
		case tar.TypeReg:
			files = append(files, header.Name)
			Expect(err).NotTo(HaveOccurred())
		case tar.TypeDir:
		default:
			Expect(err).NotTo(HaveOccurred())
		}
	}
	return files
}
