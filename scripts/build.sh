#!/bin/sh
set -e

# Get API_HOST from environment or use default
API_HOST=${API_HOST:-tracer.zaruchevskiy.ru}

echo "Building with API_HOST: $API_HOST"

# Update the @host annotation in main.go
sed -i.bak "s|^// @host .*|// @host $API_HOST|" /app/cmd/api/main.go

# Generate swagger docs
echo "Generating Swagger documentation..."
swag init -g cmd/api/main.go -o docs

# Build the application
echo "Building application..."
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ton-tracer-api ./cmd/api

echo "Build complete!"
