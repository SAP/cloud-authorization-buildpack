package client

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
)

//go:generate mockgen --build_flags=--mod=mod --destination=../supply/client_mock_test.go --package=supply github.com/SAP/cloud-authorization-buildpack/pkg/client AMSClient
type AMSClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewAMSClient() (AMSClient, error) {
	crt, err := tls.LoadX509KeyPair(os.Getenv("CF_INSTANCE_CERT"), os.Getenv("CF_INSTANCE_KEY"))
	if err != nil {
		return nil, fmt.Errorf("could not load cf certs %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.Certificates = []tls.Certificate{crt}
	amsClient :=
		http.Client{
			Transport: transport,
		}
	return &amsClient, nil
}
