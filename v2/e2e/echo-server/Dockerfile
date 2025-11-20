FROM ghcr.io/cybozu/golang:1.24-noble AS build-env

ARG TARGETARCH

COPY . /workdir
WORKDIR /workdir

RUN GOARCH=${TARGETARCH} CGO_ENABLED=0 GOOS=linux go build -o echotest main.go

FROM ghcr.io/cybozu/ubuntu:24.04 

COPY --from=build-env /workdir/echotest /usr/local/bin/echotest
