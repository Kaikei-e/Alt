# Build stage
FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o alt-butterfly-facade main.go

# Minimal distroless image with non-root user
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /build/alt-butterfly-facade /alt-butterfly-facade

USER nonroot:nonroot

EXPOSE 9200

ENTRYPOINT ["/alt-butterfly-facade"]
