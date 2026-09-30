FROM --platform=$BUILDPLATFORM ghcr.io/cybozu/golang:1.26-noble as build-env

ARG TARGETARCH
ARG GOPROXY

COPY . /workdir
WORKDIR /workdir

# The netrc secret carries Takumi Guard credentials when built from CI; local builds work without it.
RUN --mount=type=secret,id=netrc,target=/root/.netrc \
    GOPROXY=${GOPROXY} go mod download

RUN GOPROXY=off make build GOARCH=${TARGETARCH}

FROM --platform=$TARGETPLATFORM ghcr.io/cybozu/ubuntu:24.04

# https://docs.github.com/en/packages/managing-container-images-with-github-container-registry/connecting-a-repository-to-a-container-image#connecting-a-repository-to-a-container-image-on-the-command-line
LABEL org.opencontainers.image.source https://github.com/cybozu-go/coil

RUN apt-get update \
    && apt-get install -y --no-install-recommends netbase kmod iptables nftables iproute2 conntrack \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build-env /workdir/work /usr/local/coil

ENV PATH /usr/local/coil:$PATH
