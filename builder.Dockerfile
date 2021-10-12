FROM golang:1.16

RUN go get github.com/cloudfoundry/libbuildpack@80929621d4
RUN cd pkg/mod/github.com/cloudfoundry/libbuildpack\@v0.0.0-20210726164432-80929621d448/packager/buildpack-packager && go install
