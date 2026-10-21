FROM ghcr.io/cybozu/golang:1.27.3.1_resolute@sha256:56354d624535147898457543f53b6b42bebd671817e0ec322b610b1370c98f2c AS build-env

ARG TARGETARCH

COPY . /workdir
WORKDIR /workdir

RUN GOARCH=${TARGETARCH} CGO_ENABLED=0 GOOS=linux go build -o echotest main.go

FROM ghcr.io/cybozu/ubuntu:26.04.20261021@sha256:4d40515de878b3276fa0b0fd6ee4818f7dc729d579d3a211dae0763bfad4ff02 

COPY --from=build-env /workdir/echotest /usr/local/bin/echotest
