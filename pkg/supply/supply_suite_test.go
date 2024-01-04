package supply_test

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"testing"
)

func TestSupply(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Supply Suite")
}
