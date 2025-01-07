FROM golang:1.23.1-alpine AS builder

# Use muslc for static libs
ARG BUILD_TAGS="muslc"

RUN apk add --no-cache --update openssh git make build-base linux-headers libc-dev \
                                pkgconfig zeromq-dev musl-dev alpine-sdk libsodium-dev \
                                libzmq-static libsodium-static gcc

# Build
WORKDIR /go/src/github.com/babylonlabs-io/covenant-emulator/covenant-signer
# Cache dependencies
COPY ../go.mod ../go.sum /go/src/github.com/babylonlabs-io/covenant-emulator/covenant-signer/
# Copy the rest of the files
COPY ./ /go/src/github.com/babylonlabs-io/covenant-emulator/covenant-signer/

# Cosmwasm - Download correct libwasmvm version
SHELL ["/bin/ash", "-eo", "pipefail", "-c"]
RUN WASMVM_VERSION=$(go list -m github.com/CosmWasm/wasmvm/v2 | cut -d ' ' -f 2) && \
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc."$(uname -m)".a \
    -O /lib/libwasmvm_muslc."$(uname -m)".a && \
    # verify checksum
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/"$WASMVM_VERSION"/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc."$(uname -m)".a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc."$(uname -m)" | cut -d ' ' -f 1)


RUN BUILD_TAGS=$BUILD_TAGS \
    LINK_STATICALLY=true \
    make build

# FINAL IMAGE
FROM alpine:3.16 AS run

RUN addgroup --gid 1138 -S covenant-signer && adduser --uid 1138 -S covenant-signer -G covenant-signer

RUN apk add bash curl jq

COPY --from=builder /go/src/github.com/babylonlabs-io/covenant-emulator/covenant-signer/build/covenant-signer /bin/covenant-signer

WORKDIR /home/covenant-signer
RUN chown -R covenant-signer /home/covenant-signer
USER covenant-signer
