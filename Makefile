VERSION:=$(shell cat VERSION)

build: build-image
	docker run --rm -v "${PWD}:/src" -w /src buildpack-packager \
	  buildpack-packager build --any-stack --cached
	mv opa_buildpack-cached-v${VERSION}.zip opa_buildpack.zip
test:
	go test github.com/SAP/cloud-authorization-buildpack/...
lint:
	golangci-lint run

build-image:
	docker build -t buildpack-packager -f builder.Dockerfile .

reuse-lint:
	pipx run reuse lint

.PHONY: build build-image lint test
