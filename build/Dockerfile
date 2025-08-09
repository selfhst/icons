# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy source code first
COPY go.mod main.go ./

# Download dependencies and build
RUN go mod tidy && \
    go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server .

# Final stage - use scratch for minimal image
FROM scratch

# Add ca-certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/server /server

# Create non-root user
USER 65534:65534

EXPOSE 4050

ENTRYPOINT ["/server"]