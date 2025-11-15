# TON Transaction Tracer

A comprehensive toolkit for tracing and analyzing TON blockchain transaction chains, available as both a CLI tool and REST API microservice.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue.svg)](https://golang.org/)
[![TON](https://img.shields.io/badge/TON-Blockchain-blue.svg)](https://ton.org/)

## Overview

TON Transaction Tracer helps you understand complex transaction chains on the TON blockchain by recursively following transaction trees, calculating balance changes, and tracking network fees across all involved accounts. Perfect for analyzing DeFi operations like Stonfi swaps, jetton transfers, or any multi-step transaction flow.

## Features

- **Transaction Chain Tracing**: Recursively follow complete transaction trees from start to finish
- **Balance Analysis**: Track TON balance changes and network fees for all accounts in the chain
- **DNS Resolution**: Support for .ton and .t.me domain names
- **Interactive CLI**: Select from recent transactions with a user-friendly interface
- **REST API**: Production-ready microservice with Swagger documentation
- **Testnet Support**: Test with TON testnet before using mainnet
- **No External APIs**: Uses only `tonutils-go` and public liteservers
- **Export Capabilities**: JSON export for further analysis

## Quick Start

### CLI Tool

```bash
# Clone the repository
git clone https://github.com/YarikYar/tracerr.git
cd tracerr

# Build the CLI tool
go build -o ton-tracer

# Interactive mode - select from recent transactions
./ton-tracer -addr EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf

# Trace specific transaction
./ton-tracer \
  -addr EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf \
  -hash 0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229 \
  -lt 63633416000001

# Use TON DNS
./ton-tracer -addr foundation.ton

# Export to JSON
./ton-tracer -addr foundation.ton -export results.json
```

### REST API

```bash
# Switch to API branch
git checkout api-service

# Start with Docker Compose
docker-compose up -d

# Access Swagger UI
open http://localhost:8080/swagger/index.html

# Trace a transaction via API
curl -X POST http://localhost:8080/api/v1/trace \
  -H "Content-Type: application/json" \
  -d '{
    "address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
    "scan_depth": 1000
  }'
```

## Repository Branches

This repository contains multiple branches for different use cases:

### `master` - Production CLI Tool
The main branch contains the stable CLI tool with all essential features:
- Interactive transaction selection
- DNS resolution support
- Balance tracking for all accounts
- JSON export
- Configurable scan depth

### `cli-client` - Enhanced CLI Features
Enhanced version of the CLI with additional features:
- Improved DNS resolution
- Better error handling
- Enhanced user experience

### `api-service` - REST API Microservice
Full-featured REST API for integrating transaction tracing into your applications:
- Swagger/OpenAPI documentation
- Docker support with docker-compose
- Health check endpoints
- CORS enabled
- Production-ready with proper error handling

## Installation

### Prerequisites

- Go 1.25.3 or higher
- Internet connection (for accessing TON liteservers)
- Docker (optional, for API service)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/YarikYar/tracerr.git
cd tracerr

# Build CLI tool
go build -o ton-tracer

# Or build API service
git checkout api-service
go build -o ton-tracer-api ./cmd/api
```

### Using Docker (API Service)

```bash
# Switch to API branch
git checkout api-service

# Run with docker-compose
docker-compose up -d

# Or build and run manually
docker build -t ton-tracer-api .
docker run -d -p 8080:8080 ton-tracer-api
```

## Usage

### CLI Tool Options

```
-addr string
    Transaction address (required) - supports TON DNS (.ton, .t.me)

-hash string
    Transaction hash (hex) - optional for interactive mode

-lt uint
    Logical time - optional for interactive mode

-scan-depth int
    Number of transactions to scan per account (default: 100)

-recent int
    Number of recent transactions to show for selection (default: 10)

-testnet
    Use testnet instead of mainnet

-quiet
    Quiet mode: only show final summary

-export string
    Export results to JSON file (e.g., output.json)
```

### CLI Examples

**Interactive Mode:**
```bash
# Show recent transactions and select one to trace
./ton-tracer -addr EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf

# Use TON DNS domain
./ton-tracer -addr foundation.ton

# Show more recent transactions
./ton-tracer -addr foundation.ton -recent 20
```

**Direct Transaction Tracing:**
```bash
# Trace specific transaction
./ton-tracer \
  -addr EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf \
  -hash 0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229 \
  -lt 63633416000001 \
  -scan-depth 1000
```

**Export and Quiet Mode:**
```bash
# Export results and suppress trace output
./ton-tracer \
  -addr foundation.ton \
  -quiet \
  -export stonfi-swap.json
```

**Testnet Usage:**
```bash
# Use testnet for testing
./ton-tracer -addr <testnet-address> -testnet
```

### API Examples

See [README_API.md](README_API.md) on the `api-service` branch for comprehensive API documentation.

**Quick API Example:**
```bash
# Trace latest transaction
curl -X POST http://localhost:8080/api/v1/trace \
  -H "Content-Type: application/json" \
  -d '{"address": "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"}'

# Resolve DNS
curl -X POST http://localhost:8080/api/v1/dns/resolve \
  -H "Content-Type: application/json" \
  -d '{"domain": "foundation.ton"}'
```

## Output Format

### CLI Output

```
============================================================
TRACE SUMMARY
============================================================
Total transactions traced: 9
Original sender: EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf

Account Balance Changes & Network Fees:
------------------------------------------------------------

Account: EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf [ORIGINAL]
  Balance Change: -0.115324419 TON
  Network Fees:   0.005692355 TON

Account: EQBxj...
  Balance Change: 0.109632064 TON
  Network Fees:   0.001234567 TON

============================================================
```

### JSON Export Format

```json
{
  "timestamp": "2025-11-15T10:30:00Z",
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

## How It Works

### Transaction Matching

The tracer uses multiple strategies to match child transactions:

1. **Primary**: CreatedLT + Source Address (most accurate)
2. **Fallback**: CreatedLT only (when source address unavailable)
3. **Account State**: Uses LastTxLT and LastTxHash when available
4. **Pagination**: Scans configurable depth to find matching transactions

### Balance Calculation

- **Incoming**: Messages received by the account (positive)
- **Outgoing**: Messages sent by the account (negative)
- **Fees**: Network fees tracked separately
- **Net Balance**: Incoming - Outgoing - Fees

### Scanning Process

1. Fetch initial transaction
2. Extract outgoing messages
3. For each outgoing message:
   - Find destination account transactions
   - Match by CreatedLT and source address
   - Recursively trace matched transactions
4. Aggregate balance changes and fees

## Use Cases

- **DeFi Analysis**: Understand complex swap operations on Stonfi or DeDust
- **Transaction Debugging**: Debug multi-step smart contract interactions
- **Fee Estimation**: Calculate total fees for complex operations
- **Audit Trail**: Track fund flows across multiple accounts
- **API Integration**: Integrate transaction tracing into your dApp

## Technical Details

### Dependencies

- `github.com/xssnick/tonutils-go` - TON blockchain interaction
- `github.com/gin-gonic/gin` - API framework (api-service branch)
- `github.com/swaggo/swag` - Swagger documentation (api-service branch)

### Architecture

**CLI Tool:**
- Single binary with no external dependencies
- Direct connection to TON liteservers
- Recursive transaction traversal
- In-memory balance tracking

**API Service:**
```
api/
├── handlers/    # HTTP request handlers
└── models/      # Request/response models
cmd/
└── api/        # API server entry point
pkg/
└── tracer/     # Shared tracing logic
docs/           # Auto-generated Swagger docs
```

### Performance Considerations

- **Scan Depth**: Higher values find more transactions but take longer
- **Network Latency**: Performance depends on liteserver connection
- **Transaction Complexity**: Deep transaction trees take longer to trace
- **Pagination**: Automatic batching optimizes API calls

## Troubleshooting

### Common Issues

**"Transaction not found"**
- Verify hash and LT are correct
- Check if using correct network (mainnet/testnet)
- Try interactive mode instead

**"Failed to connect to liteservers"**
- Check internet connection
- Verify firewall settings
- TON liteservers may be temporarily unavailable

**DNS resolution fails**
- Domain may not have a wallet record
- Verify domain spelling
- Check if domain exists on the correct network

**Balance mismatch with explorer**
- Increase scan depth to find all transactions
- Check if all child transactions were matched
- Verify transaction tree depth didn't exceed limit

### Debug Tips

```bash
# Use verbose mode (non-quiet) to see matching details
./ton-tracer -addr foundation.ton

# Increase scan depth for complex chains
./ton-tracer -addr foundation.ton -scan-depth 500

# Export to JSON for detailed analysis
./ton-tracer -addr foundation.ton -export debug.json
```

## Development

### Running Tests

```bash
go test ./...
```

### Building for Production

```bash
# CLI
CGO_ENABLED=0 go build -ldflags="-s -w" -o ton-tracer

# API
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ton-tracer-api ./cmd/api
```

### Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Roadmap

- [ ] Jetton balance tracking
- [ ] NFT transfer tracing
- [ ] GraphQL API
- [ ] Web UI dashboard
- [ ] Historical transaction replay
- [ ] Export to CSV/Excel
- [ ] Transaction visualization
- [ ] Batch processing
- [ ] Webhook notifications

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [tonutils-go](https://github.com/xssnick/tonutils-go) - Excellent TON SDK
- TON Foundation - For the amazing TON blockchain
- TON Community - For support and feedback

## Support

- **Issues**: [GitHub Issues](https://github.com/YarikYar/tracerr/issues)
- **Discussions**: [GitHub Discussions](https://github.com/YarikYar/tracerr/discussions)
- **TON Dev Chat**: [Telegram](https://t.me/tondev)

## Links

- [TON Documentation](https://docs.ton.org/)
- [TON Explorer](https://tonviewer.com/)
- [tonutils-go](https://github.com/xssnick/tonutils-go)
- [TON Testnet](https://testnet.tonviewer.com/)

---

Made with ❤️ for the TON ecosystem
