package main

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SAP/cloud-authorization-buildpack/resources/testdata"
)

func Test_copyCertToDisk(t *testing.T) {
	depsDir, err := os.MkdirTemp("", "test")
	assert.NoError(t, err)

	crtBytes := []byte("hello cert")
	keyBytes := []byte("hello key")
	err = copyCertToDisk(depsDir, crtBytes, keyBytes)
	assert.NoError(t, err)

	crt, err := os.ReadFile(path.Join(depsDir, "ias.crt"))
	assert.NoError(t, err)
	assert.Equal(t, crtBytes, crt)
	key, err := os.ReadFile(path.Join(depsDir, "ias.key"))
	assert.Equal(t, keyBytes, key)
	assert.NoError(t, err)
}

func Test_loadCert(t *testing.T) {
	tests := []struct {
		name     string
		testdata string
		wantCert []byte
		wantKey  []byte
		wantErr  assert.ErrorAssertionFunc
	}{
		{name: "IAS X509", testdata: testdata.EnvWithIASAuthX509, wantCert: []byte("identity-cert-payload"), wantKey: []byte("identity-key-payload"), wantErr: assert.NoError},
		{name: "IAS X509 Expired", testdata: testdata.EnvWithIASAuthX509Expired, wantCert: nil, wantKey: nil, wantErr: assert.Error},
		{name: "Env with megaclite", testdata: testdata.EnvWithMegaclite, wantCert: nil, wantKey: nil, wantErr: assertErrorIsMegacliteMode},
		{name: "User-provided IAS X509", testdata: testdata.EnvWithUserProvidedIAS, wantCert: []byte("identity-cert-payload"), wantKey: []byte("identity-key-payload"), wantErr: assert.NoError},
		{name: "Service binding missing", testdata: testdata.EnvWithAllMissing, wantCert: nil, wantKey: nil, wantErr: assert.Error},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VCAP_SERVICES", tt.testdata)

			gotCert, gotKey, err := loadCert()
			if !tt.wantErr(t, err, "loadCert()") {
				return
			}
			assert.Equalf(t, tt.wantCert, gotCert, "loadCert()")
			assert.Equalf(t, tt.wantKey, gotKey, "loadCert()")
		})
	}
}

func assertErrorIsMegacliteMode(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
	return assert.ErrorIs(t, ErrMegacliteMode, err, msgAndArgs)
}
