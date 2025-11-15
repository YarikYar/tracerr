package tracer

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/xssnick/tonutils-go/address"
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
	Transactions  []*TransactionInfo
	AccountStats  map[string]*AccountStats
	OriginalAddr  string
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
	tracker := &BalanceTracker{
		Transactions: make([]*TransactionInfo, 0),
		AccountStats: make(map[string]*AccountStats),
	}

	if len(tx.AccountAddr) > 0 {
		txAddr := address.NewAddress(0, 0, tx.AccountAddr)
		tracker.OriginalAddr = txAddr.String()
	}

	err := traceTransactionChain(ctx, api, tx, tracker, 0, config)
	if err != nil {
		return nil, err
	}

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

	// Track stats for this account
	if txAddr != nil {
		addrStr := txAddr.String()
		stats, exists := tracker.AccountStats[addrStr]
		if !exists {
			stats = &AccountStats{
				Address:       addrStr,
				BalanceChange: big.NewInt(0),
				Fees:          big.NewInt(0),
			}
			tracker.AccountStats[addrStr] = stats
		}
		stats.BalanceChange.Add(stats.BalanceChange, balanceChange)
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

			destAddr := intMsg.DstAddr
			destTxs := scanAccountTransactions(ctx, api, destAddr, config.ScanDepth)
			if len(destTxs) == 0 {
				continue
			}

			var currentTxAddr *address.Address
			if len(tx.AccountAddr) > 0 {
				currentTxAddr = address.NewAddress(0, 0, tx.AccountAddr)
			}

			for _, destTx := range destTxs {
				if destTx.IO.In != nil && destTx.IO.In.MsgType == tlb.MsgTypeInternal {
					inMsg := destTx.IO.In.AsInternal()

					createdLTMatch := inMsg.CreatedLT == intMsg.CreatedLT
					srcAddrMatch := false
					if currentTxAddr != nil && inMsg.SrcAddr != nil {
						srcAddrMatch = inMsg.SrcAddr.Equals(currentTxAddr)
					}

					if createdLTMatch && srcAddrMatch {
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, config)
						if err != nil {
							return err
						}
						break
					} else if createdLTMatch {
						err = traceTransactionChain(ctx, api, destTx, tracker, depth+1, config)
						if err != nil {
							return err
						}
						break
					}
				}
			}
		}
	}

	return nil
}

func scanAccountTransactions(ctx context.Context, api ton.APIClientWrapped, addr *address.Address, limit int) []*tlb.Transaction {
	var allTxs []*tlb.Transaction

	master, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get master block: %v", err)
		return allTxs
	}

	account, err := api.GetAccount(ctx, master, addr)
	if err != nil || account == nil || !account.IsActive || account.LastTxLT == 0 {
		txs, err := api.ListTransactions(ctx, addr, uint32(limit), 0, nil)
		if err == nil {
			return txs
		}
		return allTxs
	}

	currentLT := account.LastTxLT
	currentHash := account.LastTxHash
	batchSize := uint32(15)

	for len(allTxs) < limit {
		txs, err := api.ListTransactions(ctx, addr, batchSize, currentLT, currentHash)
		if err != nil {
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

	return allTxs
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
