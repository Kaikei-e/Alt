FROM golang:1.27.1-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o readonly-proxy main.go

FROM scratch
COPY --from=builder /app/readonly-proxy /readonly-proxy
ENTRYPOINT ["/readonly-proxy"]
EXPOSE 2375
