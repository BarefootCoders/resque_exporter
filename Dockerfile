FROM golang:alpine AS build-env
MAINTAINER Jason Berlinsky <jason@barefootcoders.com>

ENV VERSION "0.0.3"

RUN apk add --update --no-cache \
  make \
  git

RUN mkdir -p /go/src/github.com/BarefootCoders/resque_exporter
WORKDIR /go/src/github.com/BarefootCoders/resque_exporter

ADD . /go/src/github.com/BarefootCoders/resque_exporter

RUN make installdeps
RUN make clean
RUN make build-linux-amd64
RUN ls /go/src/github.com/BarefootCoders/resque_exporter/bin/resque_exporter_linux_amd64_$VERSION

FROM alpine
MAINTAINER Jason Berlinsky <jason@barefootcoders.com>

ENV VERSION "0.0.3"

WORKDIR /app
COPY --from=build-env /go/src/github.com/BarefootCoders/resque_exporter/bin/resque_exporter_linux_amd64_$VERSION /app/
ENTRYPOINT ./resque_exporter_linux_amd64_$VERSION
