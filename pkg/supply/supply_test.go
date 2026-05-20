package supply_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo" //nolint
	. "github.com/onsi/gomega" //nolint

	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply/env"
	"github.com/SAP/cloud-authorization-buildpack/pkg/uploader"
	"github.com/SAP/cloud-authorization-buildpack/resources/testdata"
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
		uploadReqSpy = nil
		certSpy = nil
		keySpy = nil
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
		Expect(os.Setenv("CF_STACK", "cflinuxfs4")).To(Succeed())
		wd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		buildpackDir := path.Join(filepath.Dir(filepath.Dir(wd)))

		args := []string{buildDir, "", depsDir, depsIdx}
		bps := libbuildpack.NewStager(args, logger, &libbuildpack.Manifest{})
		supplier = &supply.Supplier{
			Stager:           bps,
			Log:              logger,
			BuildpackDir:     buildpackDir,
			BuildpackVersion: "UNIT-TEST",
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
		Context("and credential type is x509", func() {
			BeforeEach(func() {
				vcapServices = testdata.EnvWithIASAuthX509
				os.Setenv("AMS_DCL_ROOT", "/policies")
				os.Setenv("VCAP_APPLICATION", "{\"application_name\":\"unit-tests-appname\"}")
			})
			It("should succeed", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(string(keySpy)).To(Equal("identity-key-payload"))
				Expect(string(certSpy)).To(Equal("identity-cert-payload"))
			})
			It("sets the ams instance id http header when uploading the bundle", func() {
				Expect(supplier.Run()).To(Succeed())
				expectedValue := []string{"00000000-3b4d-4c41-9e5b-9aee7bfa6348"}
				Expect(uploadReqSpy.Header).Should(HaveKeyWithValue(env.HeaderInstanceID, expectedValue))
			})
			It("sets the buildpack version as User-Agent Header", func() {
				Expect(supplier.Run()).To(Succeed())
				expectedValue := []string{"cloud-authorization-buildpack/UNIT-TEST"}
				Expect(uploadReqSpy.Header).Should(HaveKeyWithValue("User-Agent", expectedValue))
			})
			It("sets the cf app name as upload header", func() {
				Expect(supplier.Run()).To(Succeed())
				expectedValue := []string{"unit-tests-appname"}
				Expect(uploadReqSpy.Header).Should(HaveKeyWithValue("X-Appname", expectedValue))
			})
			It("uploads to the correct URL", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(uploadReqSpy.URL.String()).To(Equal("https://mytenant.accounts400.ondemand.com/sap/ams/v1/ams-instances/00000000-3b4d-4c41-9e5b-9aee7bfa6348/dcl-upload"))
			})
			It("uploads DCL and json files in a bundle", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(uploadReqSpy.Body).NotTo(BeNil())
				files := getTgzFileNames(uploadReqSpy.Body)
				Expect(files).To(ContainElements("myPolicies0/policy0.dcl", "myPolicies1/policy1.dcl", "schema.dcl"))
				Expect(files).NotTo(ContainElements("non-dcl-file.xyz", ContainSubstring("data.json")))
			})

			It("does NOT write any OPA sidecar artifacts", func() {
				Expect(supplier.Run()).To(Succeed())
				Expect(filepath.Join(depDir, "opa_config.yml")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(depDir, "launch.yml")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(depDir, "opa")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(depDir, "bin", "cert-to-disk")).NotTo(BeAnExistingFile())
				Expect(filepath.Join(buildDir, ".profile.d", "0000_opa_env.sh")).NotTo(BeAnExistingFile())
			})

			It("writes a deprecation profile.d script", func() {
				Expect(supplier.Run()).To(Succeed())
				scriptPath := filepath.Join(buildDir, ".profile.d", "0000_ams_deprecation.sh")
				Expect(scriptPath).To(BeARegularFile())
				expectIsExecutable(scriptPath)
				content, err := os.ReadFile(scriptPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("cloud-authorization-buildpack is DEPRECATED"))
				Expect(string(content)).To(ContainSubstring("policies-deployer"))
				Expect(string(content)).To(ContainSubstring("**WARNING**"))
			})

			When("AMS_DCL_ROOT is not set", func() {
				BeforeEach(func() {
					Expect(os.Unsetenv("AMS_DCL_ROOT")).To(Succeed())
					Expect(os.Unsetenv("AMS_SERVICE")).To(Succeed())
				})
				It("creates a warning and does not upload", func() {
					Expect(supplier.Run()).To(Succeed())
					Expect(writtenLogs.String()).To(ContainSubstring("upload no authorization data"))
					Expect(uploadReqSpy).To(BeNil())
				})
				It("still writes the deprecation profile.d script", func() {
					Expect(supplier.Run()).To(Succeed())
					Expect(filepath.Join(buildDir, ".profile.d", "0000_ams_deprecation.sh")).To(BeARegularFile())
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
	When("VCAP_SERVICES is empty (and upload requested)", func() {
		BeforeEach(func() {
			os.Setenv("AMS_DCL_ROOT", "/policies")
		})
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
		It("should upload using the CF instance certificate via megaclite", func() {
			Expect(supplier.Run()).To(Succeed())
			Expect(uploadReqSpy.Host).To(Equal("megaclite.host"))
			Expect(string(keySpy)).To(Equal("cf-instance-key-payload"))
			Expect(string(certSpy)).To(Equal("cf-instance-cert-payload"))
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
	Describe("deprecation banner", func() {
		BeforeEach(func() {
			vcapServices = testdata.EnvWithIASAuthX509
			os.Setenv("AMS_DCL_ROOT", "/policies")
		})
		It("logs the deprecation warning at staging time", func() {
			supply.PrintDeprecation(logger)
			Expect(writtenLogs.String()).To(ContainSubstring("**WARNING**"))
			Expect(writtenLogs.String()).To(ContainSubstring("cloud-authorization-buildpack is DEPRECATED"))
			Expect(writtenLogs.String()).To(ContainSubstring("policies-deployer"))
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
