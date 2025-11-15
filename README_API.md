# TON Transaction Tracer API

A production-ready REST API microservice for tracing TON blockchain transactions and analyzing balance flows across accounts.

## Features

- 🔍 **Transaction Tracing**: Trace complete transaction chains with balance analysis
- 🌐 **DNS Resolution**: Resolve .ton and .t.me domains to wallet addresses
- 📊 **Account Analytics**: Get balance changes and network fees for all involved accounts
- 📝 **Swagger Documentation**: Interactive API documentation at `/swagger/index.html`
- 🐳 **Docker Support**: Fully containerized with Docker and docker-compose
- 🏥 **Health Checks**: Built-in health check endpoints for monitoring
- 🔄 **Testnet Support**: Optional testnet mode for development and testing

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Start the API service
docker-compose up -d

# Access the API
open http://localhost:8080

# Access Swagger documentation
open http://localhost:8080/swagger/index.html

# View logs
docker-compose logs -f

# Stop the service
docker-compose down
```

### Using Docker

```bash
# Build the image
docker build -t ton-tracer-api .

# Run the container
docker run -d -p 8080:8080 ton-tracer-api

# Run in testnet mode
docker run -d -p 8080:8080 ton-tracer-api -testnet
```

### Running Locally

```bash
# Install dependencies
go mod download

# Run the API server
go run cmd/api/main.go

# Run in testnet mode
go run cmd/api/main.go -testnet

# Run on custom port
go run cmd/api/main.go -port 3000
```

## API Endpoints

### Health Check
```
GET /health
```

### Trace Transaction
```
POST /api/v1/trace
Content-Type: application/json

{
  "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
  "hash": "0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229",
  "lt": 63633416000001,
  "scan_depth": 1000
}
```

**Response:**
```json
{
  "success": true,
  "original_sender": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
  "total_transactions": 9,
  "accounts": [
    {
      "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
      "balance_change_ton": "-0.115324419",
      "network_fees_ton": "0.005692355"
    }
  ]
}
```

### Trace Latest Transaction
Omit `hash` and `lt` to automatically trace the latest transaction:
```
POST /api/v1/trace
Content-Type: application/json

{
  "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
  "scan_depth": 1000
}
```

### Resolve DNS
```
POST /api/v1/dns/resolve
Content-Type: application/json

{
  "domain": "foundation.ton"
}
```

**Response:**
```json
{
  "success": true,
  "domain": "foundation.ton",
  "address": "EQAU_6rW5f5P3eZCANkI2zCgIJl6PqDLuHVs2LmNaLfSXwIa"
}
```

### Get Recent Transactions
```
POST /api/v1/transactions/recent
Content-Type: application/json

{
  "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
  "count": 10
}
```

**Response:**
```json
{
  "success": true,
  "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
  "transactions": [
    {
      "hash": "0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229",
      "lt": 63633416000001,
      "timestamp": 1699876543,
      "amount": "0.05"
    }
  ]
}
```

## Example Usage

### Using cURL

```bash
# Trace a transaction
curl -X POST http://localhost:8080/api/v1/trace \
  -H "Content-Type: application/json" \
  -d '{
    "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
    "hash": "0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229",
    "lt": 63633416000001
  }'

# Resolve a TON domain
curl -X POST http://localhost:8080/api/v1/dns/resolve \
  -H "Content-Type: application/json" \
  -d '{"domain": "foundation.ton"}'

# Get recent transactions
curl -X POST http://localhost:8080/api/v1/transactions/recent \
  -H "Content-Type: application/json" \
  -d '{"address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf", "count": 5}'

# Health check
curl http://localhost:8080/health
```

### Using HTTPie

```bash
# Trace a transaction
http POST localhost:8080/api/v1/trace \
  address="EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf" \
  hash="0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229" \
  lt:=63633416000001

# Resolve DNS
http POST localhost:8080/api/v1/dns/resolve \
  domain="foundation.ton"
```

### Using Python

```python
import requests

# Trace a transaction
response = requests.post('http://localhost:8080/api/v1/trace', json={
    'address': 'EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf',
    'hash': '0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229',
    'lt': 63633416000001,
    'scan_depth': 1000
})

data = response.json()
print(f"Total transactions: {data['total_transactions']}")
for account in data['accounts']:
    print(f"{account['address']}: {account['balance_change_ton']} TON")
```

### Using JavaScript/Node.js

```javascript
// Trace a transaction
const response = await fetch('http://localhost:8080/api/v1/trace', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    address: 'EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf',
    hash: '0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229',
    lt: 63633416000001
  })
});

const data = await response.json();
console.log(`Traced ${data.total_transactions} transactions`);
```

## Configuration

### Environment Variables

- `GIN_MODE`: Set to `release` for production (default: `debug`)
- Port is configured via command-line flag: `-port 8080`
- Network mode via flag: `-testnet`

### Command-Line Flags

- `-port`: Server port (default: `8080`)
- `-testnet`: Use testnet instead of mainnet

## Docker Compose Profiles

The docker-compose setup includes optional profiles:

```bash
# Run mainnet only (default)
docker-compose up

# Run both mainnet and testnet
docker-compose --profile testnet up

# Run testnet only
docker-compose --profile testnet up ton-tracer-api-testnet
```

## Development

### Project Structure

```
.
├── api/
│   ├── handlers/       # HTTP request handlers
│   └── models/         # Request/response models
├── cmd/
│   └── api/           # API server entry point
├── pkg/
│   └── tracer/        # Core tracing logic
├── docs/              # Swagger documentation (auto-generated)
├── Dockerfile         # Container definition
├── docker-compose.yml # Multi-container setup
└── README_API.md      # This file
```

### Regenerating Swagger Docs

```bash
# Install swag
go install github.com/swaggo/swag/cmd/swag@latest

# Generate docs
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

### Building

```bash
# Build binary
go build -o ton-tracer-api ./cmd/api

# Build Docker image
docker build -t ton-tracer-api .

# Build for production
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ton-tracer-api ./cmd/api
```

## Monitoring

### Health Check

The service includes a built-in health check endpoint at `/health`:

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "healthy",
  "version": "1.0.0"
}
```

### Docker Health Checks

The Docker container includes automatic health checks that run every 30 seconds:

```bash
# Check container health status
docker ps

# View health check logs
docker inspect --format='{{json .State.Health}}' ton-tracer-api | jq
```

## Production Deployment

### Recommended Setup

1. Use Docker Compose for easy deployment
2. Place behind a reverse proxy (nginx, traefik, etc.)
3. Enable HTTPS with Let's Encrypt
4. Set up monitoring and alerting
5. Configure log aggregation

### Example nginx Configuration

```nginx
server {
    listen 80;
    server_name api.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Scaling

For high-traffic scenarios:

```yaml
# docker-compose-scale.yml
version: '3.8'

services:
  ton-tracer-api:
    build: .
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: '1'
          memory: 512M
    ports:
      - "8080-8082:8080"
```

## API Reference

Full API documentation is available at `/swagger/index.html` when the server is running.

## Troubleshooting

### Common Issues

**Connection timeout to TON network:**
- Check internet connectivity
- Verify firewall settings
- Try testnet mode if mainnet is unavailable

**Transaction not found:**
- Verify the transaction hash and LT are correct
- Ensure the address is correct
- Try increasing scan depth

**Domain resolution fails:**
- Verify the domain exists and has a wallet record
- Check if the domain is on the correct network (mainnet/testnet)

### Logs

```bash
# Docker Compose logs
docker-compose logs -f ton-tracer-api

# Docker logs
docker logs -f ton-tracer-api

# Follow last 100 lines
docker logs --tail 100 -f ton-tracer-api
```

## License

MIT

## Support

For issues and questions, please open an issue on GitHub.
