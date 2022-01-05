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
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/open-policy-agent/opa/config"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/open-policy-agent/opa/plugins/rest"
	"gopkg.in/yaml.v2"

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/testdata"
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
			Stager:       bps,
			Manifest:     m,
			Installer:    libbuildpack.NewInstaller(m),
			Log:          logger,
			BuildpackDir: buildpackDir,
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
	When("VCAP_SERVICES contains a 'authorization' service", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithAuthorization
			os.Setenv("AMS_DCL_ROOT", "/policies")
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
				cmd := `"/home/vcap/deps/42/opa" run -s -c "/home/vcap/deps/42/opa_config.yml" -l 'error' -a '127.0.0.1:9888' --skip-version-check`
				Expect(ld.Processes[0].Command).To(Equal(cmd))
				Expect(ld.Processes[0].Limits.Memory).To(Equal(100))
				Expect(writtenLogs.String()).To(ContainSubstring("writing launch.yml"))
			})
		})
		It("creates the correct opa Config", func() {
			Expect(supplier.Run()).To(Succeed())
			Expect(writtenLogs.String()).To(ContainSubstring("writing opa Config"))

			rawConfig, err := os.ReadFile(filepath.Join(depDir, "opa_config.yml"))
			Expect(err).NotTo(HaveOccurred())
			cfg, err := config.ParseConfig(rawConfig, "testId")
			Expect(err).NotTo(HaveOccurred())

			var serviceKey string
			By("specifying the correct bundle options", func() {
				var bundleConfig map[string]bundle.Source
				err = json.Unmarshal(cfg.Bundles, &bundleConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(bundleConfig).To(HaveKey("SAP"))
				serviceKey = bundleConfig["SAP"].Service
				Expect(serviceKey).NotTo(BeEmpty())
				Expect(bundleConfig["SAP"].Resource).To(Equal("SAP.tar.gz"))
				Expect(*bundleConfig["SAP"].Polling.MinDelaySeconds).To(Equal(int64(10)))
				Expect(*bundleConfig["SAP"].Polling.MaxDelaySeconds).To(Equal(int64(20)))
			})
			By("specifying proper s3 rest Config", func() {
				var restConfig map[string]rest.Config
				err = json.Unmarshal(cfg.Services, &restConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(restConfig).To(HaveKey(serviceKey))
				Expect(restConfig[serviceKey].Credentials.ClientTLS).To(BeNil())
				Expect(restConfig[serviceKey].Credentials.S3Signing).NotTo(BeNil())
				Expect(restConfig[serviceKey].URL).To(Equal("https://s3-eu-central-1.amazonaws.com/my-bucket"))
			})
			By("enabling the OPA dcn plugin", func() {
				enabled, ok := cfg.Plugins["dcl"]
				Expect(ok).To(BeTrue())
				Expect(string(enabled)).To(Equal(`true`))
			})
		})
		It("creates the correct env vars", func() {
			Expect(supplier.Run()).To(Succeed())
			env, err := os.ReadFile(path.Join(buildDir, ".profile.d", "0000_opa_env.sh"))
			Expect(err).NotTo(HaveOccurred())
			expectIsExecutable(path.Join(buildDir, ".profile.d", "0000_opa_env.sh"))
			Expect(string(env)).To(ContainSubstring(`export OPA_URL=http://localhost:9888`))
			Expect(string(env)).To(ContainSubstring("export AWS_ACCESS_KEY_ID=myawstestaccesskeyid"))
		})
		It("provides the OPA executable", func() {
			Expect(supplier.Run()).To(Succeed())
			expectIsExecutable(filepath.Join(depDir, "opa"))
		})
		It("uploads DCL and json files in a bundle", func() {
			Expect(supplier.Run()).To(Succeed())
			Expect(uploadReqSpy.Body).NotTo(BeNil())
			files := getTgzFileNames(uploadReqSpy.Body)
			Expect(files).To(ContainElements("myPolicies0/policy0.dcl", "myPolicies1/policy1.dcl", "schema.dcl"))
			Expect(files).NotTo(ContainElements("data.json.license", "non-dcl-file.xyz", ContainSubstring("data.json")))
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
				Expect(files).NotTo(ContainElements("data.json.license", "non-dcl-file.xyz", ContainSubstring("data.json")))
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
			Context("401", func() {
				BeforeEach(func() {
					mockAMSClient = NewMockAMSClient(mockCtrl)
					mockAMSClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
						uploadReqSpy = req
						return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("your policy is broken"))}, nil
					}).AnyTimes()

				})
				It("should log the response body", func() {
					err := supplier.Run()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("your policy is broken"))
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
	When("service_name is set", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithAuthorizationDev
			os.Setenv("AMS_DCL_ROOT", "/policies")
			os.Setenv("AMS_SERVICE", "authorization-dev")
		})
		It("should succeed", func() {
			Expect(supplier.Run()).To(Succeed())
		})
		When("the bundle URL is set", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithUPSBundleURL
			})
			It("should succeed", func() {
				Expect(supplier.Run()).To(Succeed())
			})
		})

	})
	When("the bound AMS service is user-provided", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithUserProvidedAuthorization
			os.Setenv("AMS_DCL_ROOT", "/policies")
		})
		It("should succeed", func() {
			Expect(supplier.Run()).To(Succeed())
		})
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
		Context("and credential type is x509", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithIASAuthX509
				os.Setenv("AMS_DCL_ROOT", "/policies")
			})
			It("should succeed", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(filepath.Join(depDir, "launch.yml")).To(BeARegularFile())
				Expect(uploadReqSpy.Host).To(Equal("ams.url.from.identity"))
				//TODO: Test certificates
				Expect(keySpy).NotTo(BeNil())  //To(Equal(path.Join(supplier.Stager.DepDir(), "ias.key")))
				Expect(certSpy).NotTo(BeNil()) //To(Equal(path.Join(supplier.Stager.DepDir(), "ias.crt")))
			})
			Context("the bundle gateway url is set", func() {
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
						Expect(restConfig["bundle_storage"].URL).To(Equal("https://my-bundle-gateway.org/some/path"))
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
