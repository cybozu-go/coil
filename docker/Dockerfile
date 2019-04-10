# Coil container

# Stage1: set link
FROM quay.io/cybozu/ubuntu:18.04 AS stage1
WORKDIR /coil
COPY hypercoil /coil/
RUN ln -s hypercoil coil \
    && ln -s hypercoil coild \
    && ln -s hypercoil coil-controller \
    && ln -s hypercoil coilctl \
    && ln -s hypercoil coil-installer

# Stage2: setup runtime container
FROM scratch
COPY LICENSE /
COPY --from=stage1 /coil /
ENTRYPOINT ["/hypercoil"]
