package uploader

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/cloudfoundry/libbuildpack"
)

type Uploader struct {
	Log           *libbuildpack.Logger
	Root          string
	Client        AMSClient
	AMSInstanceID string
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
	body, err := CreateArchive(up.Log, up.Root)
	if err != nil {
		return fmt.Errorf("could not create policy DCL.tar.gz: %w", err)
	}
	u, err := url.Parse(dstURL)
	if err != nil {
		return fmt.Errorf("invalid destination AMS URL ('%s'): %w", dstURL, err)
	}
	u.Path = path.Join(u.Path, "/sap/ams/v1/ams-instances/", up.AMSInstanceID, "/dcl-upload")
	resp, err := up.DoWithRetries(u.String(), body.Bytes(), maxRetries)
	if err != nil {
		return fmt.Errorf("could not build upload request: %w", err)
	}
	defer resp.Body.Close()
	return up.logResponse(resp)
}

const maxRetries = 9

var RetryPeriod = 10 * time.Second

func (up *Uploader) DoWithRetries(url string, body []byte, maxRetries int) (*http.Response, error) {
	resp, err := up.do(url, body)
	if err != nil {
		return nil, fmt.Errorf("DCL upload request unsuccessful: %w", err)
	}
	retries := 0
	for resp.StatusCode == http.StatusForbidden && retries < maxRetries {
		if err := drainResponseBody(resp.Body); err != nil {
			return nil, fmt.Errorf("cannot drain response body: %w", err)
		}
		up.Log.Info("certificate is not accepted (yet), retrying after  %s...", RetryPeriod.String())
		time.Sleep(RetryPeriod)
		resp, err = up.do(url, body)
		if err != nil {
			return nil, fmt.Errorf("DCL upload request unsuccessful: %w", err)
		}
	}
	return resp, nil
}

func (up *Uploader) do(url string, body []byte) (*http.Response, error) {
	r, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("could not create DCL upload request %w", err)
	}
	r.Header.Set("Content-Type", "application/gzip")
	return up.Client.Do(r)
}

func drainResponseBody(body io.ReadCloser) error {
	defer body.Close()
	_, err := io.Copy(ioutil.Discard, body)
	return err
}
