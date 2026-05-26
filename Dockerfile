# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build both binaries
RUN go build -o /app/bin/concierge ./cmd/concierge/main.go
RUN go build -o /app/bin/researcher ./cmd/researcher/main.go

# Final stage
FROM alpine:latest

WORKDIR /root/
COPY --from=builder /app/bin/concierge .
COPY --from=builder /app/bin/researcher .

# Expose ports if necessary (e.g., websocket/API)
# EXPOSE 8080

# This Dockerfile expects to be invoked with the service binary name
ENTRYPOINT ["./concierge"]
