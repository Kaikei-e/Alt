# syntax=docker/dockerfile:1.7
# Multi-stage: build a static Go binary, then assemble onto an image that
# already contains step-cli so we can mint OTTs and request certs via the
# proven CLI. The Go binary owns the lifecycle state machine, retries,
# atomic writes, metrics, and health.

FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/pki-agent ./cmd/pki-agent

# Use the same step-ca base that the existing compose sidecars pull.
# Already cached on every host that runs Alt.
FROM smallstep/step-ca:0.30.2@sha256:a2b17872915c193259b75a5474c398326f41bd199f0842093e52cf4182bc8270
USER root
COPY --from=build /out/pki-agent /usr/local/bin/pki-agent
ENTRYPOINT ["/usr/local/bin/pki-agent"]
