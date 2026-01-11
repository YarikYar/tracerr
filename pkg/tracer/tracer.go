package tracer

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"ton-tracer/pkg/jetton"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/dns"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

type TransactionInfo struct {
	Hash    string
	Address string
	LT      uint64
	Amount  *big.Int
}

type AccountStats struct {
	Address        string
	BalanceChange  *big.Int
	Fees           *big.Int
	JettonBalances map[string]*JettonBalance // jetton_wallet_address -> balance
}

type JettonBalance struct {
	JettonWallet string
	JettonMaster string
	Amount       *big.Int
	Symbol       string
	Decimals     int
}

// Known pTON (Proxy TON) addresses - these represent wrapped TON and should be filtered
// as their balance is already reflected in TON balance changes
var pTONMasters = map[string]bool{
	"EQCM3B12QK1e4yZSf8GtBRT0aLMNyEsBc_DhVfRRtOEffLez": true, // pTON v1 (STON.fi)
	"EQBnGWMCf3-FZZq1W4IWcWiGAc3PHuZ0_H-7sad2oY00o83S": true, // pTON v2 (STON.fi)
}

// IsPTON checks if the given jetton master is a pTON (Proxy TON) contract
func IsPTON(jettonMaster string) bool {
	return pTONMasters[jettonMaster]
}

type BalanceTracker struct {
	Transactions    []*TransactionInfo
	AccountStats    map[string]*AccountStats
	OriginalAddr    string
	JettonMetaCache map[string]*jetton.JettonMetadata // jetton_master -> metadata
	metaMutex       sync.RWMutex
}

type Config struct {
	ScanDepth int
	Testnet   bool
	Verbose   bool
}

// ResolveAddress resolves TON DNS domains or parses regular addresses
func ResolveAddress(ctx context.Context, api ton.APIClientWrapped, addrStr string) (*address.Address, error) {
	if strings.HasSuffix(strings.ToLower(addrStr), ".ton") || strings.HasSuffix(strings.ToLower(addrStr), ".t.me") {
		root, err := dns.GetRootContractAddr(ctx, api)
		if err != nil {
			return nil, fmt.Errorf("failed to get DNS root: %w", err)
		}

		resolver := dns.NewDNSClient(api, root)
		domain, err := resolver.Resolve(ctx, strings.ToLower(addrStr))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve DNS: %w", err)
		}

		walletAddr := domain.GetWalletRecord()
		if walletAddr == nil {
			return nil, fmt.Errorf("no wallet record found for domain: %s", addrStr)
		}

		return walletAddr, nil
	}

	return address.ParseAddr(addrStr)
}

// TraceTransaction traces a transaction chain and returns all account statistics
func TraceTransaction(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, config Config) (*BalanceTracker, error) {
	startTotal := time.Now()
	log.Printf("[TIMING] TraceTransaction started")

	tracker := &BalanceTracker{
		Transactions:    make([]*TransactionInfo, 0),
		AccountStats:    make(map[string]*AccountStats),
		JettonMetaCache: make(map[string]*jetton.JettonMetadata),
	}

	if len(tx.AccountAddr) > 0 {
		txAddr := address.NewAddress(0, 0, tx.AccountAddr)
		tracker.OriginalAddr = txAddr.String()
	}

	// Store the initial transaction LT for reference
	initialLT := tx.LT

	startChain := time.Now()
	err := traceTransactionChain(ctx, api, tx, tracker, 0, config)
	log.Printf("[TIMING] traceTransactionChain took %v", time.Since(startChain))
	if err != nil {
		return nil, err
	}

	// Scan for subsequent incoming transactions on the original address
	// This catches returns from DEX swaps (refunds, change, jetton transfers, etc.)
	if tracker.OriginalAddr != "" {
		origAddr, err := address.ParseAddr(tracker.OriginalAddr)
		if err == nil {
			// Track processed LTs to avoid duplicates
			processedLTs := make(map[uint64]bool)
			processedLTs[initialLT] = true

			// Add all LTs from recursive tracing to avoid double-processing
			for _, txInfo := range tracker.Transactions {
				processedLTs[txInfo.LT] = true
			}

			// Scan recent transactions on original address
			startScan := time.Now()
			origTxs := scanAccountTransactions(ctx, api, origAddr, config.ScanDepth)
			log.Printf("[TIMING] scanAccountTransactions(origAddr) took %v, got %d txs", time.Since(startScan), len(origTxs))
			for _, origTx := range origTxs {
				// Skip already processed transactions
				if processedLTs[origTx.LT] {
					continue
				}
				// Skip transactions before the initial one
				if origTx.LT <= initialLT {
					continue
				}
				// Stop after a reasonable time window (LT difference > 2000000 = ~200 seconds)
				// StonFi jetton transfers can arrive significantly later than the swap tx
				if origTx.LT > initialLT+2000000 {
					break
				}

				// Process any incoming transaction after our initial TX
				// This catches DEX returns, refunds, jetton notifications, etc.
				if origTx.IO.In != nil && origTx.IO.In.MsgType == tlb.MsgTypeInternal {
					inMsg := origTx.IO.In.AsInternal()
					if config.Verbose {
						srcStr := ""
						if inMsg.SrcAddr != nil {
							srcStr = inMsg.SrcAddr.String()
						}
						log.Printf("[TRACE RETURN] Found subsequent tx from %s to %s, amount=%s, LT=%d",
							srcStr, tracker.OriginalAddr, inMsg.Amount.String(), origTx.LT)
					}
					processedLTs[origTx.LT] = true
					// Process this transaction for balance changes (don't recurse deep)
					traceTransactionChain(ctx, api, origTx, tracker, 49, config)
				}
			}
		}
	}

	// Post-process jetton balances to fetch metadata
	startMeta := time.Now()
	metaCount := 0
	for _, stats := range tracker.AccountStats {
		for jettonWallet, jettonBalance := range stats.JettonBalances {
			metaCount++
			// Try to get jetton master by calling get_wallet_data on the jetton wallet
			walletAddr, err := address.ParseAddr(jettonWallet)
			if err != nil {
				continue
			}

			master, err := api.CurrentMasterchainInfo(ctx)
			if err != nil {
				continue
			}

			// Call get_wallet_data() to get balance, owner, jetton master, and wallet code
			res, err := api.RunGetMethod(ctx, master, walletAddr, "get_wallet_data")
			if err != nil {
				continue
			}

			if len(res.AsTuple()) >= 3 {
				// get_wallet_data returns: balance, owner, jetton_master, jetton_wallet_code
				jettonMasterSlice, ok := res.AsTuple()[2].(*cell.Slice)
				if ok {
					jettonMasterAddr, err := jettonMasterSlice.LoadAddr()
					if err == nil {
						jettonBalance.JettonMaster = jettonMasterAddr.String()

						// Fetch metadata
						meta := tracker.getJettonMetadata(ctx, api, jettonMasterAddr.String())
						if meta != nil {
							jettonBalance.Symbol = meta.Symbol
							jettonBalance.Decimals = meta.Decimals
						}
					}
				}
			}
		}
	}
	log.Printf("[TIMING] jetton metadata processing took %v for %d wallets", time.Since(startMeta), metaCount)
	log.Printf("[TIMING] TraceTransaction total took %v", time.Since(startTotal))

	return tracker, nil
}

func traceTransactionChain(ctx context.Context, api ton.APIClientWrapped, tx *tlb.Transaction, tracker *BalanceTracker, depth int, config Config) error {
	if depth > 50 {
		return nil
	}

	var txAddr *address.Address
	if len(tx.AccountAddr) > 0 {
		txAddr = address.NewAddress(0, 0, tx.AccountAddr)
	}

	if config.Verbose {
		txHash := ""
		if len(tx.Hash) > 0 {
			txHash = hex.EncodeToString(tx.Hash)
		}
		addrStr := ""
		if txAddr != nil {
			addrStr = txAddr.String()
		}
		log.Printf("[TRACE depth=%d] TX %s on account %s, LT=%d", depth, txHash[:16], addrStr, tx.LT)

		// Log incoming message details
		if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
			inMsg := tx.IO.In.AsInternal()
			log.Printf("  IN: from=%s amount=%s", inMsg.SrcAddr, inMsg.Amount.String())
		}

		// Log outgoing messages
		if tx.IO.Out != nil {
			outList, _ := tx.IO.Out.ToSlice()
			for i, msg := range outList {
				if msg.MsgType == tlb.MsgTypeInternal {
					intMsg := msg.AsInternal()
					log.Printf("  OUT[%d]: to=%s amount=%s", i, intMsg.DstAddr, intMsg.Amount.String())
				}
			}
		}
	}

	// Calculate total fees (transaction fees + forward fees)
	totalFees := new(big.Int).Set(tx.TotalFees.Coins.Nano())

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

	// Calculate balance change
	balanceChange := calculateBalanceChange(tx)

	// Parse jetton transfers from incoming message
	if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
		inMsg := tx.IO.In.AsInternal()
		jettonTransfer, err := jetton.ParseJettonTransfer(inMsg)
		if err == nil && jettonTransfer != nil && txAddr != nil {
			if config.Verbose {
				log.Printf("[JETTON IN] opcode=0x%08x amount=%s dest=%v",
					jettonTransfer.Opcode, jettonTransfer.Amount.String(), jettonTransfer.Destination)
			}
			// For jetton transfers, the SOURCE of the message is the jetton wallet
			// OpTransferNotification (0x7362d09c) means this account received jettons
			// The source is the account's jetton wallet
			if jettonTransfer.Opcode == jetton.OpTransferNotification && inMsg.SrcAddr != nil {
				jettonWallet := inMsg.SrcAddr.String()
				if config.Verbose {
					log.Printf("[JETTON] TransferNotification: account=%s wallet=%s amount=%s (incoming)",
						txAddr.String(), jettonWallet, jettonTransfer.Amount.String())
				}
				tracker.trackJettonTransfer(ctx, api, txAddr.String(), jettonWallet, jettonTransfer.Amount, true)
			}

			// OpInternalTransfer (0x178d4519) means this jetton wallet received jettons
			// We need to find the owner of this wallet and credit them
			if jettonTransfer.Opcode == jetton.OpInternalTransfer && txAddr != nil {
				// txAddr is the jetton wallet that received the transfer
				// Get owner by calling get_wallet_data
				owner := getJettonWalletOwner(ctx, api, txAddr)
				if owner != nil {
					jettonWallet := txAddr.String()
					if config.Verbose {
						log.Printf("[JETTON] InternalTransfer: wallet=%s owner=%s amount=%s (incoming)",
							jettonWallet, owner.String(), jettonTransfer.Amount.String())
					}
					tracker.trackJettonTransfer(ctx, api, owner.String(), jettonWallet, jettonTransfer.Amount, true)
				}
			}
		}
	}

	// Track stats for this account
	if txAddr != nil {
		addrStr := txAddr.String()
		stats, exists := tracker.AccountStats[addrStr]
		if !exists {
			stats = &AccountStats{
				Address:        addrStr,
				BalanceChange:  big.NewInt(0),
				Fees:           big.NewInt(0),
				JettonBalances: make(map[string]*JettonBalance),
			}
			tracker.AccountStats[addrStr] = stats
		}
		stats.BalanceChange.Add(stats.BalanceChange, balanceChange)
		// Only subtract fees for transactions with outgoing messages
		// (fees for incoming-only transactions are paid by the sender)
		if tx.IO.Out != nil {
			outList, err := tx.IO.Out.ToSlice()
			if err == nil && len(outList) > 0 {
				stats.BalanceChange.Sub(stats.BalanceChange, totalFees)
			}
		}
		stats.Fees.Add(stats.Fees, totalFees)
	}

	// Track transaction
	txHash := ""
	if len(tx.Hash) > 0 {
		txHash = hex.EncodeToString(tx.Hash)
	}

	addrStr := ""
	if txAddr != nil {
		addrStr = txAddr.String()
	}

	tracker.Transactions = append(tracker.Transactions, &TransactionInfo{
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

		for _, msg := range outList {
			if msg.MsgType != tlb.MsgTypeInternal {
				continue
			}

			intMsg := msg.AsInternal()
			if intMsg.DstAddr == nil {
				continue
			}

			// Parse jetton transfers from outgoing messages
			jettonTransfer, err := jetton.ParseJettonTransfer(intMsg)
			if err == nil && jettonTransfer != nil && txAddr != nil {
				if config.Verbose {
					log.Printf("[JETTON OUT] opcode=0x%08x amount=%s dest=%v to=%s",
						jettonTransfer.Opcode, jettonTransfer.Amount.String(), jettonTransfer.Destination, intMsg.DstAddr.String())
				}
				// For outgoing jetton transfers, the DESTINATION is the account's jetton wallet
				// OpTransfer (0x0f8a7ea5) means this account is sending jettons via their wallet
				if jettonTransfer.Opcode == jetton.OpTransfer && intMsg.DstAddr != nil {
					jettonWallet := intMsg.DstAddr.String()
					if config.Verbose {
						log.Printf("[JETTON] Transfer: account=%s wallet=%s amount=%s (outgoing)",
							txAddr.String(), jettonWallet, jettonTransfer.Amount.String())
					}
					tracker.trackJettonTransfer(ctx, api, txAddr.String(), jettonWallet, jettonTransfer.Amount, false)
				}
			}

			destAddr := intMsg.DstAddr
			log.Printf("[TRACE depth=%d] processing outMsg to %s, createdLT=%d", depth, destAddr.String()[:16], intMsg.CreatedLT)
			startDestScan := time.Now()
			destTxs := scanAccountTransactions(ctx, api, destAddr, config.ScanDepth)
			log.Printf("[TRACE depth=%d] scan %s took %v, got %d txs", depth, destAddr.String()[:16], time.Since(startDestScan), len(destTxs))
			if len(destTxs) == 0 {
				log.Printf("[TRACE depth=%d] no txs found for %s, skipping", depth, destAddr.String()[:16])
				continue
			}

			var currentTxAddr *address.Address
			if len(tx.AccountAddr) > 0 {
				currentTxAddr = address.NewAddress(0, 0, tx.AccountAddr)
			}

			foundMatch := false
			for i, destTx := range destTxs {
				if destTx.IO.In != nil && destTx.IO.In.MsgType == tlb.MsgTypeInternal {
					inMsg := destTx.IO.In.AsInternal()

					createdLTMatch := inMsg.CreatedLT == intMsg.CreatedLT
					srcAddrMatch := false
					if currentTxAddr != nil && inMsg.SrcAddr != nil {
						srcAddrMatch = inMsg.SrcAddr.Equals(currentTxAddr)
					}

					if createdLTMatch && srcAddrMatch {
						log.Printf("[TRACE depth=%d] found match at idx %d, recursing to depth %d", depth, i, depth+1)
						foundMatch = true
						recurseStart := time.Now()
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, config)
						log.Printf("[TRACE depth=%d] recurse took %v", depth, time.Since(recurseStart))
						if err != nil {
							return err
						}
						break
					} else if createdLTMatch {
						log.Printf("[TRACE depth=%d] found LT match at idx %d (no src match), recursing", depth, i)
						foundMatch = true
						recurseStart := time.Now()
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, config)
						log.Printf("[TRACE depth=%d] recurse took %v", depth, time.Since(recurseStart))
						if err != nil {
							return err
						}
						break
					}
				}
			}
			if !foundMatch {
				// Log more details for debugging
				log.Printf("[TRACE depth=%d] no matching tx found for createdLT=%d in %d txs, dest=%s",
					depth, intMsg.CreatedLT, len(destTxs), destAddr.String()[:20])
				if len(destTxs) > 0 {
					firstLT := destTxs[0].LT
					lastLT := destTxs[len(destTxs)-1].LT
					log.Printf("[TRACE depth=%d] scanned LT range: %d - %d, looking for createdLT=%d",
						depth, lastLT, firstLT, intMsg.CreatedLT)
				}
			}
		}
	}

	return nil
}

func scanAccountTransactions(ctx context.Context, api ton.APIClientWrapped, addr *address.Address, limit int) []*tlb.Transaction {
	scanStart := time.Now()
	addrShort := addr.String()[:16]
	log.Printf("[SCAN %s] start, limit=%d", addrShort, limit)

	var allTxs []*tlb.Transaction

	masterStart := time.Now()
	master, err := api.CurrentMasterchainInfo(ctx)
	log.Printf("[SCAN %s] CurrentMasterchainInfo took %v", addrShort, time.Since(masterStart))
	if err != nil {
		log.Printf("[SCAN %s] ERROR: Failed to get master block: %v", addrShort, err)
		return allTxs
	}

	accountStart := time.Now()
	account, err := api.GetAccount(ctx, master, addr)
	log.Printf("[SCAN %s] GetAccount took %v", addrShort, time.Since(accountStart))
	if err != nil || account == nil || !account.IsActive || account.LastTxLT == 0 {
		log.Printf("[SCAN %s] account inactive or error, trying fallback", addrShort)
		fallbackStart := time.Now()
		txs, err := api.ListTransactions(ctx, addr, uint32(limit), 0, nil)
		log.Printf("[SCAN %s] fallback ListTransactions took %v", addrShort, time.Since(fallbackStart))
		if err == nil {
			log.Printf("[SCAN %s] total took %v, got %d txs", addrShort, time.Since(scanStart), len(txs))
			return txs
		}
		log.Printf("[SCAN %s] total took %v, got 0 txs", addrShort, time.Since(scanStart))
		return allTxs
	}

	currentLT := account.LastTxLT
	currentHash := account.LastTxHash
	batchSize := uint32(15)
	batchNum := 0

	for len(allTxs) < limit {
		batchNum++
		batchStart := time.Now()
		txs, err := api.ListTransactions(ctx, addr, batchSize, currentLT, currentHash)
		log.Printf("[SCAN %s] batch %d: ListTransactions took %v, got %d txs", addrShort, batchNum, time.Since(batchStart), len(txs))
		if err != nil {
			log.Printf("[SCAN %s] batch %d error: %v", addrShort, batchNum, err)
			break
		}

		if len(txs) == 0 {
			break
		}

		allTxs = append(allTxs, txs...)

		lastTx := txs[len(txs)-1]
		if lastTx.LT == 0 {
			break
		}

		currentLT = lastTx.LT
		currentHash = lastTx.Hash

		if len(txs) < int(batchSize) {
			break
		}
	}

	log.Printf("[SCAN %s] total took %v, got %d txs in %d batches", addrShort, time.Since(scanStart), len(allTxs), batchNum)
	return allTxs
}

// getJettonMetadata fetches or retrieves cached jetton metadata
func (tracker *BalanceTracker) getJettonMetadata(ctx context.Context, api ton.APIClientWrapped, jettonMaster string) *jetton.JettonMetadata {
	tracker.metaMutex.RLock()
	meta, exists := tracker.JettonMetaCache[jettonMaster]
	tracker.metaMutex.RUnlock()

	if exists {
		return meta
	}

	// Fetch metadata
	masterAddr, err := address.ParseAddr(jettonMaster)
	if err != nil {
		log.Printf("Warning: Failed to parse jetton master address %s: %v", jettonMaster, err)
		return nil
	}

	meta, err = jetton.GetJettonMetadata(ctx, api, masterAddr)
	if err != nil {
		log.Printf("Warning: Failed to fetch jetton metadata for %s: %v", jettonMaster, err)
		return nil
	}

	tracker.metaMutex.Lock()
	tracker.JettonMetaCache[jettonMaster] = meta
	tracker.metaMutex.Unlock()

	return meta
}

// trackJettonTransfer tracks a jetton transfer for an account
func (tracker *BalanceTracker) trackJettonTransfer(ctx context.Context, api ton.APIClientWrapped, accountAddr string, jettonWallet string, amount *big.Int, isIncoming bool) {
	stats, exists := tracker.AccountStats[accountAddr]
	if !exists {
		stats = &AccountStats{
			Address:        accountAddr,
			BalanceChange:  big.NewInt(0),
			Fees:           big.NewInt(0),
			JettonBalances: make(map[string]*JettonBalance),
		}
		tracker.AccountStats[accountAddr] = stats
	}

	jettonBalance, exists := stats.JettonBalances[jettonWallet]
	if !exists {
		jettonBalance = &JettonBalance{
			JettonWallet: jettonWallet,
			Amount:       big.NewInt(0),
		}
		stats.JettonBalances[jettonWallet] = jettonBalance
	}

	// Add or subtract based on direction
	if isIncoming {
		jettonBalance.Amount.Add(jettonBalance.Amount, amount)
	} else {
		jettonBalance.Amount.Sub(jettonBalance.Amount, amount)
	}
}

// getJettonWalletOwner returns the owner address of a jetton wallet
func getJettonWalletOwner(ctx context.Context, api ton.APIClientWrapped, walletAddr *address.Address) *address.Address {
	master, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		return nil
	}

	// Call get_wallet_data() to get balance, owner, jetton_master, wallet_code
	res, err := api.RunGetMethod(ctx, master, walletAddr, "get_wallet_data")
	if err != nil {
		return nil
	}

	tuple := res.AsTuple()
	if len(tuple) < 2 {
		return nil
	}

	// get_wallet_data returns: balance, owner, jetton_master, jetton_wallet_code
	ownerSlice, ok := tuple[1].(*cell.Slice)
	if !ok {
		return nil
	}

	ownerAddr, err := ownerSlice.LoadAddr()
	if err != nil {
		return nil
	}

	return ownerAddr
}

func calculateBalanceChange(tx *tlb.Transaction) *big.Int {
	change := big.NewInt(0)

	if len(tx.AccountAddr) == 0 {
		return change
	}

	incomingAmount := big.NewInt(0)
	if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
		inMsg := tx.IO.In.AsInternal()
		incomingAmount = inMsg.Amount.Nano()
		change.Add(change, incomingAmount)
	}

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

	return change
}
