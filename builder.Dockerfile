FROM golang:1.22.5

RUN go install github.com/cloudfoundry/libbuildpack/packager/buildpack-packager@f2ae806