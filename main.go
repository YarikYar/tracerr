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
}

func main() {
	txHash := flag.String("hash", "", "Transaction hash to trace (base64 or hex)")
	txAddr := flag.String("addr", "", "Transaction address")
	txLT := flag.Uint64("lt", 0, "Transaction logical time")
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

	fmt.Printf("Tracing transaction: %s\n", *txHash)
	fmt.Printf("Address: %s\n", addr.String())
	fmt.Printf("LT: %d\n\n", *txLT)

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
	}

	err = traceTransactionChain(ctx, api, initialTx, addr, tracker, 0)
	if err != nil {
		log.Fatalf("Failed to trace transaction chain: %v", err)
	}

	// Print results
	printResults(tracker, addr)
}

func traceTransactionChain(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, originalSender *address.Address, tracker *BalanceTracker, depth int) error {
	if depth > 50 {
		fmt.Println("Max depth reached, stopping trace")
		return nil
	}

	indent := strings.Repeat("  ", depth)

	// Ensure hash is available
	var txHash string
	if len(tx.Hash) > 0 {
		txHash = hex.EncodeToString(tx.Hash)
		fmt.Printf("%sTransaction: %s\n", indent, txHash)
	} else {
		fmt.Printf("%sTransaction: <hash unavailable>\n", indent)
		txHash = ""
	}

	// Convert account address bytes to address
	var txAddr *address.Address
	if len(tx.AccountAddr) > 0 {
		txAddr = address.NewAddress(0, 0, tx.AccountAddr)
		fmt.Printf("%s  Address: %s\n", indent, txAddr.String())
	} else {
		fmt.Printf("%s  Address: <unavailable>\n", indent)
	}
	fmt.Printf("%s  LT: %d\n", indent, tx.LT)

	// Calculate balance change for this transaction
	balanceChange := calculateBalanceChange(tx, originalSender)
	if balanceChange.Cmp(big.NewInt(0)) != 0 {
		tracker.totalChange.Add(tracker.totalChange, balanceChange)
		fmt.Printf("%s  Balance Change: %s nanoTON\n", indent, balanceChange.String())
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

		fmt.Printf("%s  Outgoing messages: %d\n", indent, len(outList))

		for i, msg := range outList {
			if msg.MsgType != tlb.MsgTypeInternal {
				continue
			}

			intMsg := msg.AsInternal()
			if intMsg.DstAddr == nil {
				continue
			}

			fmt.Printf("%s    [%d] To: %s, Amount: %s nanoTON, CreatedLT: %d\n", indent, i, intMsg.DstAddr.String(), intMsg.Amount.String(), intMsg.CreatedLT)

			// Get destination account transactions
			destAddr := intMsg.DstAddr
			master, _ := api.CurrentMasterchainInfo(ctx)

			// Try to get the account state first to get the latest transaction info
			var destTxs []*tlb.Transaction
			account, err := api.GetAccount(ctx, master, destAddr)
			if err == nil && account != nil && account.IsActive && account.LastTxLT > 0 {
				// Use account's last transaction as starting point
				fmt.Printf("%s      Getting transactions from account state (LastLT: %d)...\n", indent, account.LastTxLT)
				destTxs, err = api.ListTransactions(ctx, destAddr, 100, account.LastTxLT, account.LastTxHash)
			}

			// Fallback: if account state doesn't work, try with CreatedLT+1
			if err != nil || destTxs == nil || len(destTxs) == 0 {
				fmt.Printf("%s      Fallback: trying with CreatedLT+1...\n", indent)
				destTxs, err = api.ListTransactions(ctx, destAddr, 100, intMsg.CreatedLT+1, nil)
				if err != nil {
					log.Printf("%s      Warning: Failed to get transactions for %s: %v", indent, destAddr.String(), err)
					continue
				}
			}

			fmt.Printf("%s      Scanning %d transactions...\n", indent, len(destTxs))

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
						srcAddrMatch = inMsg.SrcAddr.String() == currentTxAddr.String()
					}

					if createdLTMatch && srcAddrMatch {
						fmt.Printf("%s      -> Matched by CreatedLT %d + SrcAddr (LT: %d)\n", indent, inMsg.CreatedLT, destTx.LT)

						// Recursively trace this transaction
						err = traceTransactionChain(ctx, api, destTx, originalSender, tracker, depth+1)
						if err != nil {
							return err
						}
						found = true
						break
					} else if createdLTMatch {
						// Match by CreatedLT only (less strict)
						fmt.Printf("%s      -> Matched by CreatedLT %d only (LT: %d)\n", indent, inMsg.CreatedLT, destTx.LT)

						// Recursively trace this transaction
						err = traceTransactionChain(ctx, api, destTx, originalSender, tracker, depth+1)
						if err != nil {
							return err
						}
						found = true
						break
					}
				}
			}

			if !found {
				fmt.Printf("%s      Note: Could not find corresponding transaction\n", indent)
			}
		}
	}

	return nil
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
	isTargetAccount := txAddr.String() == targetAddrStr

	if isTargetAccount {
		// This transaction happened on the target's account
		// Incoming amount (positive)
		if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
			inMsg := tx.IO.In.AsInternal()
			change.Add(change, inMsg.Amount.Nano())
		}

		// Outgoing amounts (negative)
		if tx.IO.Out != nil {
			outList, err := tx.IO.Out.ToSlice()
			if err == nil {
				for _, msg := range outList {
					if msg.MsgType == tlb.MsgTypeInternal {
						intMsg := msg.AsInternal()
						change.Sub(change, intMsg.Amount.Nano())
					}
				}
			}
		}

		// Transaction fees (negative)
		change.Sub(change, tx.TotalFees.Coins.Nano())
	} else {
		// This transaction is on a different account, but check if target is involved

		// Check if target is receiving money in this transaction
		if tx.IO.Out != nil {
			outList, err := tx.IO.Out.ToSlice()
			if err == nil {
				for _, msg := range outList {
					if msg.MsgType == tlb.MsgTypeInternal {
						intMsg := msg.AsInternal()
						if intMsg.DstAddr != nil && intMsg.DstAddr.String() == targetAddrStr {
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
			if inMsg.SrcAddr != nil && inMsg.SrcAddr.String() == targetAddrStr {
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
	fmt.Printf("\nTotal balance change: %s nanoTON\n", tracker.totalChange.String())

	// Convert to TON
	tonAmount := new(big.Float).SetInt(tracker.totalChange)
	tonAmount.Quo(tonAmount, big.NewFloat(1e9))
	fmt.Printf("Total balance change: %s TON\n", tonAmount.String())

	fmt.Println("\nTransaction details:")
	for i, tx := range tracker.transactions {
		if tx.Amount.Cmp(big.NewInt(0)) != 0 {
			fmt.Printf("  [%d] %s: %s nanoTON\n", i+1, tx.Address, tx.Amount.String())
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}
