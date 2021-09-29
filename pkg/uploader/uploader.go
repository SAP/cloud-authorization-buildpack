package uploader

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/cloudfoundry/libbuildpack"
)

type Uploader interface {
	Upload(string, string) error
}

type uploader struct {
	log    *libbuildpack.Logger
	root   string
	client AMSClient
}

//go:generate mockgen --build_flags=--mod=mod --destination=../supply/client_mock_test.go --package=supply_test github.com/SAP/cloud-authorization-buildpack/pkg/uploader AMSClient
type AMSClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewUploader(log *libbuildpack.Logger, cert, key string) (Uploader, error) {
	crt, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("could not load cf certs %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{crt}
	amsClient :=
		&http.Client{
			Transport: transport,
		}
	return &uploader{
		log,
		"",
		amsClient,
	}, nil
}

func NewUploaderWithClient(log *libbuildpack.Logger, client AMSClient) Uploader {
	return &uploader{
		log:    log,
		client: client,
	}
}

func (up *uploader) Upload(rootDir string, dstURL string) error {
	up.root = rootDir
	up.log.Info("creating policy archive..")
	buf, err := up.createArchive(up.log, rootDir)
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
	resp, err := up.client.Do(r)
	if err != nil {
		return fmt.Errorf("DCL upload request unsuccessful: %w", err)
	}
	defer resp.Body.Close()
	return up.logResponse(resp)
}
