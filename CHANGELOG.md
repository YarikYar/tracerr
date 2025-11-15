# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive documentation for all branches
- MIT License
- Contributing guidelines
- Improved .gitignore

## [1.0.0] - 2025-11-15

### Added
- Interactive CLI mode with transaction selection
- TON DNS resolution support (.ton and .t.me domains)
- Multi-account balance tracking
- Separate network fee tracking for each account
- JSON export functionality
- Configurable scan depth
- Transaction pagination for deep chains
- Quiet mode for minimal output
- Testnet support
- Debug logging for balance calculations

### Features

#### CLI Tool (master, cli-client branches)
- Trace transaction chains recursively
- Select from recent transactions interactively
- Resolve TON DNS domains to addresses
- Export results to JSON
- Track balance changes across all accounts
- Calculate network fees separately
- Configurable scanning depth (default: 100 transactions)
- Show recent transactions (configurable count)
- Quiet mode for summary-only output

#### API Service (api-service branch)
- RESTful API with Swagger documentation
- POST /api/v1/trace - Trace transactions
- POST /api/v1/dns/resolve - Resolve DNS
- POST /api/v1/transactions/recent - Get recent transactions
- GET /health - Health check endpoint
- Docker support with docker-compose
- CORS enabled for cross-origin requests
- Mainnet and testnet support
- Auto-generated Swagger UI at /swagger/index.html

### Implementation Details
- Uses tonutils-go v1.15.5
- Advanced transaction matching (CreatedLT + Source Address)
- Account state pagination for large transaction histories
- Proper nil checking and error handling
- Hex encoding for transaction hashes
- Maximum recursion depth: 50 levels

### Changed
- Improved balance calculation to separate fees from balance changes
- Enhanced transaction matching with multiple strategies
- Better error messages and logging
- Optimized pagination for scanning deep transaction histories

### Fixed
- Address comparison in transaction matching
- Balance calculation for cross-account transfers
- Fee calculation including forward fees
- Transaction hash handling for missing hashes

## [0.2.0] - 2025-11-14

### Added
- Transaction pagination support
- Configurable scan depth
- Separate fee tracking for original sender
- Debug logging for balance calculation

### Fixed
- Address comparison logic
- Balance calculation accuracy

## [0.1.0] - 2025-11-13

### Added
- Initial release
- Basic transaction tracing
- Balance change tracking
- Connection to TON liteservers
- Recursive transaction traversal
- Command-line interface

### Features
- Trace transaction chains
- Calculate balance changes
- Follow outgoing messages
- Track transaction fees

---

## Branch History

### master
Latest stable CLI tool with all features including DNS resolution and interactive mode.

### cli-client
Enhanced CLI client with improved DNS resolution and transaction selection.

### api-service
Production-ready REST API microservice with Swagger documentation and Docker support.

---

## Future Releases

See [README.md](README.md#roadmap) for planned features and improvements.

[Unreleased]: https://github.com/yourusername/tracerr/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/yourusername/tracerr/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/yourusername/tracerr/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yourusername/tracerr/releases/tag/v0.1.0
