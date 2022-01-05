package uploader

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/cloudfoundry/libbuildpack"
)

type Uploader struct {
	Log    *libbuildpack.Logger
	Root   string
	Client AMSClient
}

//go:generate mockgen --build_flags=--mod=mod --destination=../supply/client_mock_test.go --package=supply_test github.com/SAP/cloud-authorization-buildpack/pkg/uploader AMSClient
type AMSClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func GetClient(cert, key []byte) (AMSClient, error) {
	crt, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("could not load key or certificate: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{crt}
	amsClient :=
		&http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		}
	return amsClient, nil
}

func (up *Uploader) Do(dstURL string) error {
	up.Log.Info("creating policy archive..")
	buf, err := CreateArchive(up.Log, up.Root)
	if err != nil {
		return fmt.Errorf("could not create policy DCL.tar.gz: %w", err)
	}
	u, err := url.Parse(dstURL)
	if err != nil {
		return fmt.Errorf("invalid destination AMS URL ('%s'): %w", dstURL, err)
	}
	u.Path = path.Join(u.Path, "/sap/ams/v1/bundles/SAP.tar.gz")
	r, err := http.NewRequest(http.MethodPost, u.String(), buf)
	if err != nil {
		return fmt.Errorf("could not create DCL upload request %w", err)
	}
	r.Header.Set("Content-Type", "application/gzip")
	resp, err := up.Client.Do(r)
	if err != nil {
		return fmt.Errorf("DCL upload request unsuccessful: %w", err)
	}
	defer resp.Body.Close()
	return up.logResponse(resp)
}
