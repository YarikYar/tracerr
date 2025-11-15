# Contributing to TON Transaction Tracer

Thank you for your interest in contributing to TON Transaction Tracer! This document provides guidelines and instructions for contributing.

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers and help them get started
- Focus on constructive feedback
- Respect different viewpoints and experiences

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues to avoid duplicates. When creating a bug report, include:

- **Clear title**: Use a descriptive title
- **Description**: Detailed description of the issue
- **Steps to reproduce**: Step-by-step instructions
- **Expected behavior**: What you expected to happen
- **Actual behavior**: What actually happened
- **Environment**: OS, Go version, branch name
- **Logs**: Relevant error messages or logs

Example:
```markdown
**Environment:**
- OS: Ubuntu 22.04
- Go: 1.25.3
- Branch: master
- Command: ./ton-tracer -addr foundation.ton

**Issue:**
DNS resolution fails with error: "failed to get DNS root"

**Steps to reproduce:**
1. Build the project with `go build`
2. Run `./ton-tracer -addr foundation.ton`
3. Error occurs immediately

**Expected:** Should resolve foundation.ton to wallet address
**Actual:** Error "failed to get DNS root: context deadline exceeded"
```

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When suggesting an enhancement:

- **Use a clear title** describing the enhancement
- **Provide detailed description** of the suggested enhancement
- **Explain why** this enhancement would be useful
- **Provide examples** of how it would work
- **Consider alternatives** you've thought about

### Pull Requests

1. **Fork the repository** and create your branch from `master`
2. **Follow the coding standards** (see below)
3. **Write clear commit messages** (see below)
4. **Update documentation** if needed
5. **Test your changes** thoroughly
6. **Submit a pull request**

#### Pull Request Process

1. Update the README.md with details of changes if applicable
2. Ensure your code builds and runs without errors
3. Update any relevant documentation
4. Request review from maintainers
5. Address review feedback promptly

## Development Setup

### Prerequisites

- Go 1.25.3 or higher
- Git
- Docker (for API development)

### Setting Up Development Environment

```bash
# Fork and clone the repository
git clone https://github.com/yourusername/tracerr.git
cd tracerr

# Install dependencies
go mod download

# Build the project
go build -o ton-tracer

# Run tests
go test ./...
```

### Working with Branches

**Master Branch (CLI Tool):**
```bash
git checkout master
go build -o ton-tracer
./ton-tracer -addr foundation.ton
```

**API Service Branch:**
```bash
git checkout api-service
go build -o ton-tracer-api ./cmd/api
./ton-tracer-api
```

## Coding Standards

### Go Style Guide

Follow the [Effective Go](https://golang.org/doc/effective_go) guidelines:

- Use `gofmt` to format your code
- Use meaningful variable names
- Write comments for exported functions
- Keep functions focused and small
- Handle errors properly

### Code Formatting

```bash
# Format your code
gofmt -w .

# Check for common issues
go vet ./...

# Run linter (if installed)
golangci-lint run
```

### Example Code Style

```go
// Good: Clear function with documentation
// TraceTransaction traces a transaction chain and returns balance changes
func TraceTransaction(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, config Config) (*BalanceTracker, error) {
    if tx == nil {
        return nil, fmt.Errorf("transaction cannot be nil")
    }

    tracker := &BalanceTracker{
        Transactions: make([]*TransactionInfo, 0),
        AccountStats: make(map[string]*AccountStats),
    }

    // Implementation...

    return tracker, nil
}

// Bad: Unclear function without documentation
func trace(t *tlb.Transaction) *BalanceTracker {
    bt := &BalanceTracker{}
    // Implementation...
    return bt
}
```

## Commit Message Guidelines

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

### Examples

```
feat(cli): add support for batch transaction processing

Add ability to trace multiple transactions at once by reading
addresses from a file. Supports both CSV and JSON input formats.

Closes #123
```

```
fix(tracer): correct balance calculation for failed transactions

Failed transactions were incorrectly included in balance calculations.
This fix excludes transactions with compute_exit_code != 0.

Fixes #456
```

```
docs(readme): update installation instructions for Windows

Add specific instructions for Windows users including PowerShell
commands and common troubleshooting steps.
```

## Testing Guidelines

### Writing Tests

```go
func TestResolveAddress(t *testing.T) {
    tests := []struct {
        name    string
        addr    string
        want    string
        wantErr bool
    }{
        {
            name:    "valid address",
            addr:    "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
            want:    "EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf",
            wantErr: false,
        },
        {
            name:    "invalid address",
            addr:    "invalid",
            want:    "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := parseAddress(tt.addr)
            if (err != nil) != tt.wantErr {
                t.Errorf("parseAddress() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("parseAddress() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -run TestResolveAddress
```

## Documentation

### Code Comments

- Add comments for all exported functions
- Use complete sentences
- Explain the "why" not just the "what"

```go
// CalculateBalanceChange computes the net balance change for a transaction.
// It accounts for incoming messages (positive), outgoing messages (negative),
// and excludes fees which are tracked separately in AccountStats.
func CalculateBalanceChange(tx *tlb.Transaction) *big.Int {
    // Implementation...
}
```

### README Updates

When adding features:
- Update the main README.md
- Update branch-specific READMEs if applicable
- Add examples showing how to use the new feature
- Update the feature list

## API Development (api-service branch)

### Swagger Documentation

When adding new API endpoints:

```go
// TraceTransaction godoc
// @Summary Trace a transaction chain
// @Description Traces a transaction and all its child transactions, calculating balance changes
// @Tags transactions
// @Accept json
// @Produce json
// @Param request body models.TraceRequest true "Trace request"
// @Success 200 {object} models.TraceResponse
// @Failure 400 {object} models.ErrorResponse
// @Router /api/v1/trace [post]
func (h *Handler) TraceTransaction(c *gin.Context) {
    // Implementation...
}
```

### Regenerating Swagger Docs

```bash
# Install swag
go install github.com/swaggo/swag/cmd/swag@latest

# Generate documentation
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

## Release Process

Releases are managed by maintainers. Contributors can help by:

1. Testing release candidates
2. Reporting issues found during testing
3. Updating documentation for new versions

## Questions?

- Open an issue with the `question` label
- Join TON Dev Chat on Telegram: https://t.me/tondev
- Check existing documentation

## Recognition

Contributors are recognized in:
- GitHub contributors page
- Release notes for their contributions
- Special mentions for significant contributions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to TON Transaction Tracer!
