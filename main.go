package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/dns"
)

type TransactionInfo struct {
	Hash    string
	Address string
	LT      uint64
	Amount  *big.Int
}

type AccountStats struct {
	Address       string
	BalanceChange *big.Int
	Fees          *big.Int
}

type BalanceTracker struct {
	transactions  []*TransactionInfo
	accountStats  map[string]*AccountStats // Map of address -> stats
	originalAddr  string
}

// JSON export structures
type JSONAccountStats struct {
	Address       string `json:"address"`
	BalanceChange string `json:"balance_change_ton"`
	Fees          string `json:"network_fees_ton"`
}

type JSONExport struct {
	Timestamp        string              `json:"timestamp"`
	OriginalSender   string              `json:"original_sender"`
	TotalTransactions int                `json:"total_transactions"`
	Accounts         []JSONAccountStats  `json:"accounts"`
}

var quietMode bool

func logTrace(format string, args ...interface{}) {
	if !quietMode {
		fmt.Printf(format, args...)
	}
}

func resolveAddress(ctx context.Context, api ton.APIClientWrapped, addrStr string) (*address.Address, error) {
	// Check if it's a .ton domain
	if strings.HasSuffix(strings.ToLower(addrStr), ".ton") || strings.HasSuffix(strings.ToLower(addrStr), ".t.me") {
		fmt.Printf("Resolving TON DNS: %s...\n", addrStr)

		// Get root DNS contract address
		root, err := dns.GetRootContractAddr(ctx, api)
		if err != nil {
			return nil, fmt.Errorf("failed to get DNS root: %w", err)
		}

		// Create DNS resolver
		resolver := dns.NewDNSClient(api, root)
		domain, err := resolver.Resolve(ctx, strings.ToLower(addrStr))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve DNS: %w", err)
		}

		// Get wallet record
		walletAddr := domain.GetWalletRecord()
		if walletAddr == nil {
			return nil, fmt.Errorf("no wallet record found for domain: %s", addrStr)
		}

		fmt.Printf("✓ Resolved to: %s\n", walletAddr.String())
		return walletAddr, nil
	}

	// Regular address parsing
	return address.ParseAddr(addrStr)
}

func main() {
	txHash := flag.String("hash", "", "Transaction hash to trace (base64 or hex) - optional if you want to select from recent txs")
	txAddr := flag.String("addr", "", "Transaction address (required)")
	txLT := flag.Uint64("lt", 0, "Transaction logical time - optional if you want to select from recent txs")
	scanDepth := flag.Int("scan-depth", 100, "Number of transactions to scan per account (default 100)")
	recentCount := flag.Int("recent", 10, "Number of recent transactions to show for selection (default 10)")
	testnet := flag.Bool("testnet", false, "Use testnet instead of mainnet")
	exportJSON := flag.String("export", "", "Export results to JSON file (e.g., output.json)")
	flag.BoolVar(&quietMode, "quiet", false, "Quiet mode: only show final summary")
	flag.Parse()

	if *txAddr == "" {
		log.Fatal("Please provide transaction address using -addr flag")
	}

	ctx := context.Background()

	// Connect to TON liteserver
	client := liteclient.NewConnectionPool()

	// Select config based on network
	configURL := "https://ton.org/global.config.json"
	if *testnet {
		configURL = "https://ton.org/testnet-global.config.json"
		fmt.Println("Using TESTNET")
	}

	// Connect to public liteservers
	err := client.AddConnectionsFromConfigUrl(ctx, configURL)
	if err != nil {
		log.Fatalf("Failed to connect to liteservers: %v", err)
	}

	api := ton.NewAPIClient(client, ton.ProofCheckPolicySecure).WithRetry()

	// Parse address (support both regular addresses and .ton domains)
	addr, err := resolveAddress(ctx, api, *txAddr)
	if err != nil {
		log.Fatalf("Failed to parse/resolve address: %v", err)
	}
	fmt.Printf("Using address: %s\n", addr.String())

	var initialTx *tlb.Transaction

	// If hash and LT are not provided, show recent transactions for selection
	if *txHash == "" || *txLT == 0 {
		fmt.Printf("\nFetching recent %d transactions for address: %s\n\n", *recentCount, addr.String())

		// Get account state first to get LastTxLT and LastTxHash
		master, err := api.CurrentMasterchainInfo(ctx)
		if err != nil {
			log.Fatalf("Failed to get master block: %v", err)
		}

		account, err := api.GetAccount(ctx, master, addr)
		if err != nil {
			log.Fatalf("Failed to get account: %v", err)
		}

		if account == nil || !account.IsActive {
			log.Fatal("Account is not active or does not exist")
		}

		// Use LastTxLT and LastTxHash from account state
		txs, err := api.ListTransactions(ctx, addr, uint32(*recentCount), account.LastTxLT, account.LastTxHash)
		if err != nil {
			log.Fatalf("Failed to get transactions: %v", err)
		}

		if len(txs) == 0 {
			log.Fatal("No transactions found for this address")
		}

		// Display transactions
		fmt.Println("Recent transactions:")
		fmt.Println(strings.Repeat("-", 80))
		for i, tx := range txs {
			txHashStr := "N/A"
			if len(tx.Hash) > 0 {
				txHashStr = hex.EncodeToString(tx.Hash)[:16] + "..."
			}

			timestamp := time.Unix(int64(tx.Now), 0).Format("2006-01-02 15:04:05")

			// Get amount info
			amountInfo := "N/A"
			if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
				inMsg := tx.IO.In.AsInternal()
				tonAmount := new(big.Float).SetInt(inMsg.Amount.Nano())
				tonAmount.Quo(tonAmount, big.NewFloat(1e9))
				amountInfo = fmt.Sprintf("+%s TON", tonAmount.Text('f', 6))
			}

			fmt.Printf("[%2d] Hash: %s | LT: %d | Time: %s | %s\n",
				i+1, txHashStr, tx.LT, timestamp, amountInfo)
		}
		fmt.Println(strings.Repeat("-", 80))

		// Get user selection
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nSelect transaction number (1-", len(txs), ") or 'q' to quit: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" || input == "Q" {
			fmt.Println("Exiting...")
			os.Exit(0)
		}

		selection, err := strconv.Atoi(input)
		if err != nil || selection < 1 || selection > len(txs) {
			log.Fatalf("Invalid selection: %s", input)
		}

		initialTx = txs[selection-1]
		*txLT = initialTx.LT
		if len(initialTx.Hash) > 0 {
			*txHash = hex.EncodeToString(initialTx.Hash)
		}

		fmt.Printf("\nSelected transaction: %s (LT: %d)\n\n", *txHash, *txLT)
	} else {
		// Original behavior: use provided hash and LT
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

		initialTx = txs[0]

		// Verify hash matches
		if len(initialTx.Hash) > 0 {
			actualHash := hex.EncodeToString(initialTx.Hash)
			if actualHash != *txHash {
				log.Printf("Warning: Transaction hash mismatch. Expected: %s, Got: %s", *txHash, actualHash)
			}
		} else {
			log.Printf("Warning: Transaction hash is not available")
		}
	}

	// Trace the transaction chain
	tracker := &BalanceTracker{
		transactions: make([]*TransactionInfo, 0),
		accountStats: make(map[string]*AccountStats),
		originalAddr: addr.String(),
	}

	err = traceTransactionChain(ctx, api, initialTx, tracker, 0, *scanDepth)
	if err != nil {
		log.Fatalf("Failed to trace transaction chain: %v", err)
	}

	// Print results
	printResults(tracker)

	// Export to JSON if requested
	if *exportJSON != "" {
		err := exportToJSON(tracker, *exportJSON)
		if err != nil {
			log.Fatalf("Failed to export to JSON: %v", err)
		}
		fmt.Printf("\n✓ Results exported to: %s\n", *exportJSON)
	}
}

func traceTransactionChain(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, tracker *BalanceTracker, depth int, scanDepth int) error {
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

	// Calculate total fees (transaction fees + forward fees from outgoing messages)
	totalFees := new(big.Int).Set(tx.TotalFees.Coins.Nano())

	// Add forward fees from outgoing messages
	if tx.IO.Out != nil {
		outList, err := tx.IO.Out.ToSlice()
		if err == nil {
			for _, msg := range outList {
				if msg.MsgType == tlb.MsgTypeInternal {
					intMsg := msg.AsInternal()
					totalFees.Add(totalFees, intMsg.FwdFee.Nano())
					totalFees.Add(totalFees, intMsg.IHRFee.Nano())
				}
			}
		}
	}

	logTrace("%s  Fees: %s nanoTON\n", indent, totalFees.String())

	// Calculate balance change for this transaction
	balanceChange := calculateBalanceChange(tx)
	if balanceChange.Cmp(big.NewInt(0)) != 0 {
		logTrace("%s  Balance Change: %s nanoTON\n", indent, balanceChange.String())
	}

	// Track stats for this account
	if txAddr != nil {
		addrStr := txAddr.String()
		stats, exists := tracker.accountStats[addrStr]
		if !exists {
			stats = &AccountStats{
				Address:       addrStr,
				BalanceChange: big.NewInt(0),
				Fees:          big.NewInt(0),
			}
			tracker.accountStats[addrStr] = stats
		}
		stats.BalanceChange.Add(stats.BalanceChange, balanceChange)
		stats.Fees.Add(stats.Fees, totalFees)
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
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, scanDepth)
						if err != nil {
							return err
						}
						found = true
						break
					} else if createdLTMatch {
						// Match by CreatedLT only (less strict)
						logTrace("%s      -> Matched by CreatedLT %d only (LT: %d)\n", indent, inMsg.CreatedLT, destTx.LT)

						// Recursively trace this transaction
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, scanDepth)
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

func calculateBalanceChange(tx *tlb.Transaction) *big.Int {
	change := big.NewInt(0)

	// Convert transaction address to comparable format
	if len(tx.AccountAddr) == 0 {
		return change
	}

	txAddr := address.NewAddress(0, 0, tx.AccountAddr)

	// This transaction happened on this account
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

	// NOTE: Fees are NOT included in balance change
	// They are tracked separately in AccountStats
	feesAmount := tx.TotalFees.Coins.Nano()

	// Debug logging
	if len(txAddr.String()) >= 8 {
		log.Printf("Balance calc for %s: incoming=%s, outgoing=%s, fees=%s, net=%s",
			txAddr.String()[:8], incomingAmount.String(), outgoingAmount.String(), feesAmount.String(), change.String())
	}

	return change
}

func printResults(tracker *BalanceTracker) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TRACE SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total transactions traced: %d\n", len(tracker.transactions))
	fmt.Printf("Original sender: %s\n\n", tracker.originalAddr)

	// Display stats for each account
	fmt.Println("Account Balance Changes & Network Fees:")
	fmt.Println(strings.Repeat("-", 60))

	// Sort accounts to show original sender first
	var accounts []string
	for addr := range tracker.accountStats {
		accounts = append(accounts, addr)
	}

	// Move original sender to front
	for i, addr := range accounts {
		if addr == tracker.originalAddr {
			accounts[0], accounts[i] = accounts[i], accounts[0]
			break
		}
	}

	for _, addr := range accounts {
		stats := tracker.accountStats[addr]

		// Balance change INCLUDING fees (like TONViewer shows)
		balanceWithFees := new(big.Int).Set(stats.BalanceChange)
		balanceWithFees.Sub(balanceWithFees, stats.Fees)

		// Convert to TON
		balanceTon := new(big.Float).SetInt(balanceWithFees)
		balanceTon.Quo(balanceTon, big.NewFloat(1e9))

		feesTon := new(big.Float).SetInt(stats.Fees)
		feesTon.Quo(feesTon, big.NewFloat(1e9))

		// Mark original sender
		marker := ""
		if addr == tracker.originalAddr {
			marker = " [ORIGINAL]"
		}

		fmt.Printf("\nAccount: %s%s\n", addr, marker)
		fmt.Printf("  Balance Change: %s TON\n", balanceTon.Text('f', 9))
		fmt.Printf("  Network Fees:   %s TON\n", feesTon.Text('f', 9))
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
}

func exportToJSON(tracker *BalanceTracker, filename string) error {
	var accounts []JSONAccountStats

	for addr, stats := range tracker.accountStats {
		// Balance change INCLUDING fees (like TONViewer shows)
		balanceWithFees := new(big.Int).Set(stats.BalanceChange)
		balanceWithFees.Sub(balanceWithFees, stats.Fees)

		// Convert to TON
		balanceTon := new(big.Float).SetInt(balanceWithFees)
		balanceTon.Quo(balanceTon, big.NewFloat(1e9))

		feesTon := new(big.Float).SetInt(stats.Fees)
		feesTon.Quo(feesTon, big.NewFloat(1e9))

		accounts = append(accounts, JSONAccountStats{
			Address:       addr,
			BalanceChange: balanceTon.Text('f', 9),
			Fees:          feesTon.Text('f', 9),
		})
	}

	export := JSONExport{
		Timestamp:        time.Now().Format(time.RFC3339),
		OriginalSender:   tracker.originalAddr,
		TotalTransactions: len(tracker.transactions),
		Accounts:         accounts,
	}

	jsonData, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
