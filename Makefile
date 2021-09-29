test:
	go test github.com/SAP/cloud-authorization-buildpack/...
lint:
	golangci-lint run
.PHONY: test
