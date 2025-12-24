# Build stage
FROM golang:1.25.5-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code (needed for local build without network)
COPY *.go ./

# Download dependencies (if network is available) or use vendored deps
RUN go mod download || true

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o slackcompose .

# Final stage - using scratch for minimal image
FROM scratch

# Copy the binary from builder
COPY --from=builder /build/slackcompose /slackcompose

# Copy CA certificates for HTTPS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the application
ENTRYPOINT ["/slackcompose"]
