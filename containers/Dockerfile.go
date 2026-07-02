# --- Multi-stage build for Go Helix RPC service ---
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .

# Compile fully statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-s -w" -o server server/main.go

# --- Final minimal secure stage ---
FROM scratch

# Copy TLS certificates for secure connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy compiled binary and JSON config
COPY --from=builder /app/server /server
COPY --from=builder /app/helix.json /helix.json

EXPOSE 8080
ENTRYPOINT ["/server"]
