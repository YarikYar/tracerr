.PHONY: help swagger build run test docker-build docker-up docker-down clean

# Default target
help:
	@echo "Available targets:"
	@echo "  swagger       - Generate Swagger documentation"
	@echo "  build         - Build the application binary"
	@echo "  run           - Run the application locally"
	@echo "  test          - Run tests"
	@echo "  docker-build  - Build Docker images"
	@echo "  docker-up     - Start services with docker-compose"
	@echo "  docker-down   - Stop services"
	@echo "  clean         - Remove build artifacts"

# Generate Swagger docs
swagger:
	@echo "Generating Swagger documentation..."
	@swag init -g cmd/api/main.go -o docs

# Build the application
build: swagger
	@echo "Building ton-tracer-api..."
	@go build -o bin/ton-tracer-api ./cmd/api

# Run the application locally
run: swagger
	@echo "Running ton-tracer-api..."
	@go run ./cmd/api

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	@docker-compose build

# Start services
docker-up:
	@echo "Starting services..."
	@docker-compose up -d

# Stop services
docker-down:
	@echo "Stopping services..."
	@docker-compose down

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -rf docs/
	@go clean
