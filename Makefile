VERSION:=$(shell cat VERSION)

build: build-image
	docker run --rm -v "${PWD}:/src" -w /src buildpack-packager \
	  buildpack-packager build --stack cflinuxfs3 --cached
	mv opa_buildpack-cached-cflinuxfs3-v${VERSION}.zip opa_buildpack.zip
test:
	go test github.com/SAP/cloud-authorization-buildpack/...
lint:
	golangci-lint run

build-image:
	docker build -t buildpack-packager -f builder.Dockerfile .

.PHONY: build build-image lint test
