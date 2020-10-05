FROM quay.io/cybozu/ubuntu:18.04

# https://docs.github.com/en/packages/managing-container-images-with-github-container-registry/connecting-a-repository-to-a-container-image#connecting-a-repository-to-a-container-image-on-the-command-line
LABEL org.opencontainers.image.source https://github.com/cybozu-go/coil

RUN apt-get update \
    && apt-get install -y --no-install-recommends netbase kmod iptables \
    && rm -rf /var/lib/apt/lists/*

COPY /work /usr/local/coil

ENV PATH /usr/local/coil:$PATH
