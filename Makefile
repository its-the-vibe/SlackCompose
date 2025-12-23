.PHONY: build test clean run docker-build docker-run fmt vet

# Build the application
build:
	go build -o slackcompose .

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f slackcompose

# Run the application
run:
	go run .

# Build Docker image
docker-build:
	docker build -t slackcompose:latest .

# Run with docker-compose
docker-run:
	docker-compose up -d

# Stop docker-compose
docker-stop:
	docker-compose down

# Format code
fmt:
	gofmt -w .

# Run go vet
vet:
	go vet ./...

# Run linters and checks
lint: fmt vet
	go mod tidy

# Install dependencies
deps:
	go mod download

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application binary"
	@echo "  test         - Run tests"
	@echo "  clean        - Remove build artifacts"
	@echo "  run          - Run the application locally"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Start service with docker-compose"
	@echo "  docker-stop  - Stop docker-compose services"
	@echo "  fmt          - Format Go code"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run all linters and format code"
	@echo "  deps         - Download dependencies"
	@echo "  help         - Show this help message"
