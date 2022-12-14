FROM golang:1.19

RUN go install github.com/cloudfoundry/libbuildpack/packager/buildpack-packager@80929621d4