package supply_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"

	"code.cloudfoundry.org/buildpackapplifecycle/buildpackrunner/resources"
	"github.com/SAP/cloud-authorization-buildpack/pkg/supply"
	"github.com/cloudfoundry/libbuildpack"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"gopkg.in/yaml.v2"
)

//go:generate mockgen -source=supply.go --destination=mocks_test.go --package=supply_test

var _ = Describe("Supply", func() {
	var (
		err          error
		depsDir      string
		depsIdx      string
		depDir       string
		supplier     *supply.Supplier
		logger       *libbuildpack.Logger
		mockCtrl     *gomock.Controller
		mockManifest *MockManifest
		buffer       *bytes.Buffer
	)

	BeforeEach(func() {
		depsDir, err = os.MkdirTemp("", "test")
		Expect(err).To(BeNil())

		depsIdx = "42"
		depDir = filepath.Join(depsDir, depsIdx)

		err = os.MkdirAll(depDir, 0755)
		Expect(err).To(BeNil())

		buffer = new(bytes.Buffer)
		logger = libbuildpack.NewLogger(buffer)

		mockCtrl = gomock.NewController(GinkgoT())
		mockManifest = NewMockManifest(mockCtrl)
	})

	JustBeforeEach(func() {
		args := []string{"", "", depsDir, depsIdx}
		bps := libbuildpack.NewStager(args, logger, &libbuildpack.Manifest{})

		supplier = &supply.Supplier{
			Stager:   bps,
			Manifest: mockManifest,
			Log:      logger,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()

		err = os.RemoveAll(depsDir)
		Expect(err).To(BeNil())
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
			Expect(ld.Processes[0].Command).To(Equal(path.Join(depDir, "start_opa.sh")))
			Expect(ld.Processes[0].Limits.Memory).To(Equal(100))
			Expect(buffer.String()).To(ContainSubstring("writing launch.yml"))
		})
	})
	It("creates the corrent opa config", func() {
		Expect(supplier.Run()).To(Succeed())
		Expect(buffer.String()).To(ContainSubstring("writing opa config"))
		opaCfg, err := os.Open(filepath.Join(depDir, "opa_config.yml"))
		Expect(err).NotTo(HaveOccurred())

		var cfg bundle.Config
		err = json.NewDecoder(opaCfg).Decode(&cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Bundles).NotTo(BeNil())

		By("specifying proper options", func() {
			Expect(cfg.Bundles).To(HaveKey("SAP"))
			Expect(cfg.Bundles["SAP"].Service).To(Equal("s3"))
			Expect(cfg.Bundles["SAP"].Resource).To(Equal("SAP.tar.gz"))
			Expect(*cfg.Bundles["SAP"].Polling.MinDelaySeconds).To(Equal(10))
			Expect(*cfg.Bundles["SAP"].Polling.MaxDelaySeconds).To(Equal(20))
		})
	})
})
