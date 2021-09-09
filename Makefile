build:
	go build -o bin/supply ./cmd/supply

test:
	go test github.com/SAP/cloud-authorization-buildpack/...

.PHONY: build
