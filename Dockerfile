# Build stage
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY *.go ./

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o slackcompose .

# Final stage (distroless)
FROM gcr.io/distroless/static-debian13:nonroot

# Copy the binary from builder
COPY --from=builder /build/slackcompose /slackcompose

USER nonroot:nonroot

# Run the application
ENTRYPOINT ["/slackcompose"]
