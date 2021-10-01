VERSION:=$(shell cat VERSION)

build:
	buildpack-packager build --stack cflinuxfs3 --cached
	mv opa_buildpack-cached-cflinuxfs3-v${VERSION}.zip opa_buildpack.zip
test:
	go test github.com/SAP/cloud-authorization-buildpack/...
lint:
	golangci-lint run
.PHONY: build lint test
