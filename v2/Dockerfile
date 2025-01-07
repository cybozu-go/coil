FROM --platform=$BUILDPLATFORM ghcr.io/cybozu/golang:1.23-noble as build-env

ARG TARGETARCH

COPY . /workdir
WORKDIR /workdir

RUN make build GOARCH=${TARGETARCH}

FROM --platform=$TARGETPLATFORM ghcr.io/cybozu/ubuntu:24.04

# https://docs.github.com/en/packages/managing-container-images-with-github-container-registry/connecting-a-repository-to-a-container-image#connecting-a-repository-to-a-container-image-on-the-command-line
LABEL org.opencontainers.image.source https://github.com/cybozu-go/coil

RUN apt-get update \
    && apt-get install -y --no-install-recommends netbase kmod iptables iproute2 conntrack \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build-env /workdir/work /usr/local/coil

ENV PATH /usr/local/coil:$PATH
