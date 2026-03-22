# Multi-stage build: compile in Go image, run in scratch (0 OS, ~8MB total)
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY *.go ./
COPY go.mod ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o runna-sync .

# Final image: scratch = no OS, no shell, just the binary + CA certs for HTTPS
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/runna-sync /runna-sync
ENTRYPOINT ["/runna-sync"]
CMD ["sync"]
