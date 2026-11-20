FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /app
COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o readonly-proxy main.go

FROM scratch
COPY --from=builder /app/readonly-proxy /readonly-proxy
ENTRYPOINT ["/readonly-proxy"]
EXPOSE 2375
