build:
	./scripts/build.sh
test:
	go test github.com/SAP/cloud-authorization-buildpack/...
lint:
	golangci-lint run
.PHONY: build lint test
