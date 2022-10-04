FROM --platform=$BUILDPLATFORM golang:1.18-buster as build-env

ARG TARGETARCH

COPY . /workdir
WORKDIR /workdir

RUN make build GOARCH=${TARGETARCH}

FROM k8s.gcr.io/pause:3.6 as pause

# https://github.com/cybozu/ubuntu-base/tree/6507099cc3955a9ec3b7a5a18c0944d819a605a8/20.04/ubuntu-dev
FROM ubuntu:focal-20220801

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        locales \
        tzdata \
        openssl \
        netbase \
        apt-utils \
        apt-transport-https \
        libreadline8 \
        ca-certificates \
        curl \
    && apt-get -y upgrade \
    && rm -rf /var/lib/apt/lists/*

RUN locale-gen en_US.UTF-8 \
    && update-locale LANG=en_US.UTF-8 \
    && echo "Etc/UTC" > /etc/timezone \
    && dpkg-reconfigure -f noninteractive tzdata

COPY --from=pause /pause /usr/local/bin/pause

CMD ["/bin/bash"]

# https://docs.github.com/en/packages/managing-container-images-with-github-container-registry/connecting-a-repository-to-a-container-image#connecting-a-repository-to-a-container-image-on-the-command-line
LABEL org.opencontainers.image.source https://github.com/cybozu-go/coil

RUN apt-get update \
    && apt-get install -y --no-install-recommends netbase kmod iptables \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build-env /workdir/work /usr/local/coil

ENV PATH /usr/local/coil:$PATH
