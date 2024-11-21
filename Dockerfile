FROM golang:1.23 AS builder

# hadolint ignore=DL3008
RUN apt-get update && apt-get install --no-install-recommends -y ca-certificates make git bash gcc curl jq && rm -rf /var/lib/apt/lists/*

# Build
WORKDIR /go/src/github.com/babylonlabs-io/covenant-emulator
# Cache dependencies
COPY go.mod go.sum /go/src/github.com/babylonlabs-io/covenant-emulator/
RUN go mod download
# Copy the rest of the files
COPY ./ /go/src/github.com/babylonlabs-io/covenant-emulator/

RUN BUILD_TAGS=netgo \
    LDFLAGS="-w -s" \
    make build

# FINAL IMAGE
FROM debian:bookworm-slim AS run

RUN addgroup --gid 1138 --system covenant-emulator && adduser --uid 1138 --system --home /home/covenant-emulator covenant-emulator

# hadolint ignore=DL3008
RUN apt-get update && apt-get install --no-install-recommends -y ca-certificates bash curl jq wget && rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/src/github.com/babylonlabs-io/covenant-emulator/go.mod /tmp
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN WASMVM_VERSION=$(grep github.com/CosmWasm/wasmvm /tmp/go.mod | cut -d' ' -f2) && \
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm."$(uname -m)".so \
        -O /lib/libwasmvm."$(uname -m)".so && \
    # verify checksum
    wget -q https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm."$(uname -m)".so | grep $(cat /tmp/checksums.txt | grep libwasmvm."$(uname -m)" | cut -d ' ' -f 1)
RUN rm -f /tmp/go.mod

COPY --from=builder /go/src/github.com/babylonlabs-io/covenant-emulator/build/covd /bin/covd

WORKDIR /home/covenant-emulator
RUN chown -R covenant-emulator /home/covenant-emulator
USER covenant-emulator
