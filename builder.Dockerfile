FROM golang:1.26.2

RUN go install github.com/cloudfoundry/libbuildpack/packager/buildpack-packager@f2ae806