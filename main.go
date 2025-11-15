package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type TransactionInfo struct {
	Hash    string
	Address string
	LT      uint64
	Amount  *big.Int
}

type BalanceTracker struct {
	transactions []*TransactionInfo
	totalChange  *big.Int
	totalFees    *big.Int
}

var quietMode bool

func logTrace(format string, args ...interface{}) {
	if !quietMode {
		fmt.Printf(format, args...)
	}
}

func main() {
	txHash := flag.String("hash", "", "Transaction hash to trace (base64 or hex)")
	txAddr := flag.String("addr", "", "Transaction address")
	txLT := flag.Uint64("lt", 0, "Transaction logical time")
	scanDepth := flag.Int("scan-depth", 100, "Number of transactions to scan per account (default 100)")
	flag.BoolVar(&quietMode, "quiet", false, "Quiet mode: only show final summary")
	flag.Parse()

	if *txHash == "" {
		log.Fatal("Please provide transaction hash using -hash flag")
	}

	if *txAddr == "" {
		log.Fatal("Please provide transaction address using -addr flag")
	}

	if *txLT == 0 {
		log.Fatal("Please provide transaction logical time using -lt flag")
	}

	ctx := context.Background()

	// Connect to TON liteserver
	client := liteclient.NewConnectionPool()

	// Connect to public liteservers
	err := client.AddConnectionsFromConfigUrl(ctx, "https://ton.org/global.config.json")
	if err != nil {
		log.Fatalf("Failed to connect to liteservers: %v", err)
	}

	api := ton.NewAPIClient(client, ton.ProofCheckPolicySecure).WithRetry()

	// Parse address
	addr, err := address.ParseAddr(*txAddr)
	if err != nil {
		log.Fatalf("Failed to parse address: %v", err)
	}

	logTrace("Tracing transaction: %s\n", *txHash)
	logTrace("Address: %s\n", addr.String())
	logTrace("LT: %d\n\n", *txLT)

	txHashBytes, err := hex.DecodeString(*txHash)
	if err != nil {
		log.Fatalf("Failed to decode transaction hash: %v", err)
	}

	// Get the initial transaction
	txs, err := api.ListTransactions(ctx, addr, 1, *txLT, txHashBytes)
	if err != nil {
		log.Fatalf("Failed to get transaction: %v", err)
	}

	if len(txs) == 0 {
		log.Fatal("Transaction not found")
	}

	initialTx := txs[0]

	// Verify hash matches
	if len(initialTx.Hash) > 0 {
		actualHash := hex.EncodeToString(initialTx.Hash)
		if actualHash != *txHash {
			log.Printf("Warning: Transaction hash mismatch. Expected: %s, Got: %s", *txHash, actualHash)
		}
	} else {
		log.Printf("Warning: Transaction hash is not available")
	}

	// Trace the transaction chain
	tracker := &BalanceTracker{
		transactions: make([]*TransactionInfo, 0),
		totalChange:  big.NewInt(0),
		totalFees:    big.NewInt(0),
	}

	err = traceTransactionChain(ctx, api, initialTx, addr, tracker, 0, *scanDepth)
	if err != nil {
		log.Fatalf("Failed to trace transaction chain: %v", err)
	}

	// Print results
	printResults(tracker, addr)
}

func traceTransactionChain(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, originalSender *address.Address, tracker *BalanceTracker, depth int, scanDepth int) error {
	if depth > 50 {
		fmt.Println("Max depth reached, stopping trace")
		return nil
	}

	indent := strings.Repeat("  ", depth)

	// Ensure hash is available
	var txHash string
	if len(tx.Hash) > 0 {
		txHash = hex.EncodeToString(tx.Hash)
		logTrace("%sTransaction: %s\n", indent, txHash)
	} else {
		logTrace("%sTransaction: <hash unavailable>\n", indent)
		txHash = ""
	}

	// Convert account address bytes to address
	var txAddr *address.Address
	if len(tx.AccountAddr) > 0 {
		txAddr = address.NewAddress(0, 0, tx.AccountAddr)
		logTrace("%s  Address: %s\n", indent, txAddr.String())
	} else {
		logTrace("%s  Address: <unavailable>\n", indent)
	}
	logTrace("%s  LT: %d\n", indent, tx.LT)

	// Display fees for this transaction
	fees := tx.TotalFees.Coins.Nano()

	// Check if this is the original sender's account
	// Compare using Equals() to handle bounceable/non-bounceable address forms
	isOriginalSender := false
	if txAddr != nil {
		isOriginalSender = txAddr.Equals(originalSender)
	}

	if isOriginalSender {
		logTrace("%s  Fees (paid by sender): %s nanoTON\n", indent, fees.String())
		// Track total fees paid by original sender
		tracker.totalFees.Add(tracker.totalFees, fees)
	} else {
		logTrace("%s  Fees (paid by %s): %s nanoTON\n", indent, txAddr.String(), fees.String())
	}

	// Calculate balance change for this transaction
	balanceChange := calculateBalanceChange(tx, originalSender)
	if balanceChange.Cmp(big.NewInt(0)) != 0 {
		tracker.totalChange.Add(tracker.totalChange, balanceChange)
		logTrace("%s  Balance Change for sender: %s nanoTON\n", indent, balanceChange.String())
	}

	// Track this transaction
	addrStr := ""
	if txAddr != nil {
		addrStr = txAddr.String()
	}
	tracker.transactions = append(tracker.transactions, &TransactionInfo{
		Hash:    txHash,
		Address: addrStr,
		LT:      tx.LT,
		Amount:  balanceChange,
	})

	// Process outgoing messages
	if tx.IO.Out != nil {
		outList, err := tx.IO.Out.ToSlice()
		if err != nil {
			return fmt.Errorf("failed to parse out messages: %w", err)
		}

		logTrace("%s  Outgoing messages: %d\n", indent, len(outList))

		for i, msg := range outList {
			if msg.MsgType != tlb.MsgTypeInternal {
				continue
			}

			intMsg := msg.AsInternal()
			if intMsg.DstAddr == nil {
				continue
			}

			logTrace("%s    [%d] To: %s, Amount: %s nanoTON, CreatedLT: %d\n", indent, i, intMsg.DstAddr.String(), intMsg.Amount.String(), intMsg.CreatedLT)

			// Get destination account transactions
			destAddr := intMsg.DstAddr

			// Scan transactions with pagination
			destTxs := scanAccountTransactions(ctx, api, destAddr, scanDepth, indent)
			if len(destTxs) == 0 {
				log.Printf("%s      Warning: No transactions found for %s", indent, destAddr.String())
				continue
			}

			logTrace("%s      Scanned %d transactions...\n", indent, len(destTxs))

			// Find the transaction that corresponds to this message
			found := false
			var currentTxAddr *address.Address
			if len(tx.AccountAddr) > 0 {
				currentTxAddr = address.NewAddress(0, 0, tx.AccountAddr)
			}

			for _, destTx := range destTxs {
				if destTx.IO.In != nil && destTx.IO.In.MsgType == tlb.MsgTypeInternal {
					inMsg := destTx.IO.In.AsInternal()

					// Match by CreatedLT AND source address
					createdLTMatch := inMsg.CreatedLT == intMsg.CreatedLT

					srcAddrMatch := false
					if currentTxAddr != nil && inMsg.SrcAddr != nil {
						srcAddrMatch = inMsg.SrcAddr.Equals(currentTxAddr)
					}

					if createdLTMatch && srcAddrMatch {
						logTrace("%s      -> Matched by CreatedLT %d + SrcAddr (LT: %d)\n", indent, inMsg.CreatedLT, destTx.LT)

						// Recursively trace this transaction
						err = traceTransactionChain(ctx, api, destTx, originalSender, tracker, depth+1, scanDepth)
						if err != nil {
							return err
						}
						found = true
						break
					} else if createdLTMatch {
						// Match by CreatedLT only (less strict)
						logTrace("%s      -> Matched by CreatedLT %d only (LT: %d)\n", indent, inMsg.CreatedLT, destTx.LT)

						// Recursively trace this transaction
						err = traceTransactionChain(ctx, api, destTx, originalSender, tracker, depth+1, scanDepth)
						if err != nil {
							return err
						}
						found = true
						break
					}
				}
			}

			if !found {
				logTrace("%s      Note: Could not find corresponding transaction\n", indent)
			}
		}
	}

	return nil
}

func scanAccountTransactions(ctx context.Context, api ton.APIClientWrapped, addr *address.Address, limit int, indent string) []*tlb.Transaction {
	var allTxs []*tlb.Transaction

	// Get account state first
	master, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		log.Printf("%s      Warning: Failed to get master block: %v", indent, err)
		return allTxs
	}

	account, err := api.GetAccount(ctx, master, addr)
	if err != nil || account == nil || !account.IsActive || account.LastTxLT == 0 {
		// Try fallback without account state
		txs, err := api.ListTransactions(ctx, addr, uint32(limit), 0, nil)
		if err == nil {
			return txs
		}
		return allTxs
	}

	logTrace("%s      Paginating from LastLT: %d (target: %d txs)...\n", indent, account.LastTxLT, limit)

	// Paginate through transactions
	currentLT := account.LastTxLT
	currentHash := account.LastTxHash
	batchSize := uint32(15) // ListTransactions limit

	for len(allTxs) < limit {
		txs, err := api.ListTransactions(ctx, addr, batchSize, currentLT, currentHash)
		if err != nil {
			log.Printf("%s      Pagination error at LT %d: %v", indent, currentLT, err)
			break
		}

		if len(txs) == 0 {
			break
		}

		allTxs = append(allTxs, txs...)

		// Get last transaction for next iteration
		lastTx := txs[len(txs)-1]
		if lastTx.LT == 0 {
			break
		}

		// Move to next batch
		currentLT = lastTx.LT
		currentHash = lastTx.Hash

		// If we got fewer than batch size, we've reached the end
		if len(txs) < int(batchSize) {
			break
		}
	}

	logTrace("%s      Collected %d transactions total\n", indent, len(allTxs))
	return allTxs
}

func calculateBalanceChange(tx *tlb.Transaction, targetAddr *address.Address) *big.Int {
	change := big.NewInt(0)

	// Convert transaction address to comparable format
	if len(tx.AccountAddr) == 0 {
		return change
	}

	txAddr := address.NewAddress(0, 0, tx.AccountAddr)
	targetAddrStr := targetAddr.String()

	// Check if this transaction is ON the target account
	// Use Equals() to handle bounceable/non-bounceable forms
	isTargetAccount := txAddr.Equals(targetAddr)

	if isTargetAccount {
		// This transaction happened on the target's account
		// Incoming amount (positive)
		incomingAmount := big.NewInt(0)
		if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
			inMsg := tx.IO.In.AsInternal()
			incomingAmount = inMsg.Amount.Nano()
			change.Add(change, incomingAmount)
		}

		// Outgoing amounts (negative)
		outgoingAmount := big.NewInt(0)
		if tx.IO.Out != nil {
			outList, err := tx.IO.Out.ToSlice()
			if err == nil {
				for _, msg := range outList {
					if msg.MsgType == tlb.MsgTypeInternal {
						intMsg := msg.AsInternal()
						change.Sub(change, intMsg.Amount.Nano())
						outgoingAmount.Add(outgoingAmount, intMsg.Amount.Nano())
					}
				}
			}
		}

		// Transaction fees (negative)
		feesAmount := tx.TotalFees.Coins.Nano()
		change.Sub(change, feesAmount)

		// Debug logging
		log.Printf("Balance calc for %s: incoming=%s, outgoing=%s, fees=%s, net=%s",
			targetAddrStr[:8], incomingAmount.String(), outgoingAmount.String(), feesAmount.String(), change.String())
	} else {
		// This transaction is on a different account, but check if target is involved

		// Check if target is receiving money in this transaction
		if tx.IO.Out != nil {
			outList, err := tx.IO.Out.ToSlice()
			if err == nil {
				for _, msg := range outList {
					if msg.MsgType == tlb.MsgTypeInternal {
						intMsg := msg.AsInternal()
						if intMsg.DstAddr != nil && intMsg.DstAddr.Equals(targetAddr) {
							// Target is receiving money (positive)
							change.Add(change, intMsg.Amount.Nano())
						}
					}
				}
			}
		}

		// Check if target sent this transaction (source of incoming message)
		if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
			inMsg := tx.IO.In.AsInternal()
			if inMsg.SrcAddr != nil && inMsg.SrcAddr.Equals(targetAddr) {
				// This transaction was triggered by target's outgoing message
				// (already counted in target's transaction, so don't double count)
			}
		}
	}

	return change
}

func printResults(tracker *BalanceTracker, sender *address.Address) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TRACE SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total transactions traced: %d\n", len(tracker.transactions))
	fmt.Printf("Original sender: %s\n", sender.String())

	// Fees paid by sender
	fmt.Printf("\nTotal fees paid by sender: %s nanoTON\n", tracker.totalFees.String())
	feesTon := new(big.Float).SetInt(tracker.totalFees)
	feesTon.Quo(feesTon, big.NewFloat(1e9))
	fmt.Printf("Total fees paid by sender: %s TON\n", feesTon.String())

	// Balance change
	fmt.Printf("\nTotal balance change: %s nanoTON\n", tracker.totalChange.String())
	tonAmount := new(big.Float).SetInt(tracker.totalChange)
	tonAmount.Quo(tonAmount, big.NewFloat(1e9))
	fmt.Printf("Total balance change: %s TON\n", tonAmount.String())

	// Net result (should include fees already)
	fmt.Printf("\n--- Analysis ---\n")
	fmt.Printf("Fees are included in balance change\n")
	fmt.Printf("Balance change = incoming - outgoing - fees\n")

	fmt.Println("\nTransaction details:")
	for i, tx := range tracker.transactions {
		if tx.Amount.Cmp(big.NewInt(0)) != 0 {
			fmt.Printf("  [%d] %s: %s nanoTON\n", i+1, tx.Address, tx.Amount.String())
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}
