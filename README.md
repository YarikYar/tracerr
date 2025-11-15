# TON Transaction Tracer

A simple TON transaction tracer built with `tonutils-go` to analyze complex transaction chains like Stonfi swaps.

## Features

- Traces transaction chains from an initial transaction to completion
- Tracks TON balance changes for the original sender
- Uses only `tonutils-go` and public liteservers (no third-party APIs)
- Recursively follows outgoing messages to trace the entire transaction tree

## Building

```bash
go build -o ton-tracer
```

## Usage

The tracer requires three parameters:

```bash
./ton-tracer -hash <tx_hash> -addr <tx_address> -lt <logical_time>
```

### Parameters

- `-hash`: Transaction hash (base64 encoded)
- `-addr`: Transaction address (the account that initiated the transaction)
- `-lt`: Logical time of the transaction

### Example

```bash
./ton-tracer \
  -hash "Kq7G5TqPh8..." \
  -addr "EQD..." \
  -lt 12345678
```

## How to Get Transaction Parameters

You can get these parameters from TON explorers like:
- https://tonviewer.com
- https://tonscan.org

When viewing a transaction, you'll find:
- **Hash**: The transaction hash (usually shown in base64 or hex)
- **Address**: The account address involved in the transaction
- **LT**: The logical time (usually shown in transaction details)

## Output

The tracer will:
1. Connect to public TON liteservers
2. Fetch the initial transaction
3. Recursively trace all outgoing messages
4. Calculate balance changes for the original sender
5. Display a summary with total TON balance change

Example output:
```
Tracing transaction: Kq7G5TqPh8...
Address: EQD...
LT: 12345678

Transaction: Kq7G5TqPh8...
  Address: EQD...
  LT: 12345678
  Balance Change: -1000000000 nanoTON
  Outgoing messages: 2
    [0] To: EQC..., Amount: 500000000 nanoTON
    ...

============================================================
TRACE SUMMARY
============================================================
Total transactions traced: 15
Original sender: EQD...

Total balance change: -1000000000 nanoTON
Total balance change: -1 TON
============================================================
```

## Implementation Details

- Uses `tonutils-go` library for all TON interactions
- Connects to public liteservers from `https://ton.org/global.config.json`
- Implements recursive tracing by following outgoing internal messages
- Intelligent transaction matching with multiple strategies:
  1. **Best match**: CreatedLT from the message (most accurate)
  2. **Primary match**: Amount + CreatedLT
  3. **Fallback match**: Source address + CreatedLT
  4. **Alternative fallback**: Amount + approximate timing
- Only counts balance changes for the original sender address
- Includes transaction fees in balance calculations
- Maximum trace depth: 50 levels (configurable)
- Proper nil checking for all hashes and addresses
- Uses hex encoding for all transaction hashes

## Notes

- The tracer only tracks native TON balance changes (not jettons)
- Transaction matching uses message CreatedLT for accuracy, with fallbacks for edge cases
- The tool requires an internet connection to reach public liteservers
- Shows which matching strategy was used for each transaction in the trace
