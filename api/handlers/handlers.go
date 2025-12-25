package handlers

import (
	"context"
	"encoding/hex"
	"math/big"
	"net/http"

	"ton-tracer/api/models"
	"ton-tracer/pkg/tracer"

	"github.com/gin-gonic/gin"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type Handler struct {
	client *liteclient.ConnectionPool
	api    ton.APIClientWrapped
}

func NewHandler(testnet bool) (*Handler, error) {
	client := liteclient.NewConnectionPool()

	configURL := "https://ton.org/global.config.json"
	if testnet {
		configURL = "https://ton.org/testnet-global.config.json"
	}

	ctx := context.Background()
	err := client.AddConnectionsFromConfigUrl(ctx, configURL)
	if err != nil {
		return nil, err
	}

	api := ton.NewAPIClient(client, ton.ProofCheckPolicySecure).WithRetry()

	return &Handler{
		client: client,
		api:    api,
	}, nil
}

// TraceTransaction godoc
// @Summary Trace a TON transaction
// @Description Trace a transaction chain and return balance changes for all involved accounts
// @Tags transactions
// @Accept json
// @Produce json
// @Param request body models.TraceRequest true "Trace request"
// @Success 200 {object} models.TraceResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/trace [post]
func (h *Handler) TraceTransaction(c *gin.Context) {
	var req models.TraceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	ctx := context.Background()

	// Resolve address
	addr, err := tracer.ResolveAddress(ctx, h.api, req.Address)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   "Failed to resolve address: " + err.Error(),
		})
		return
	}

	scanDepth := req.ScanDepth
	if scanDepth == 0 {
		scanDepth = 100
	}

	var initialTx *tlb.Transaction

	// If hash and LT provided, fetch specific transaction
	if req.Hash != "" && req.LT != 0 {
		txHashBytes, err := hex.DecodeString(req.Hash)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Success: false,
				Error:   "Invalid transaction hash: " + err.Error(),
			})
			return
		}

		txs, err := h.api.ListTransactions(ctx, addr, 1, req.LT, txHashBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to fetch transaction: " + err.Error(),
			})
			return
		}

		if len(txs) == 0 {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Success: false,
				Error:   "Transaction not found",
			})
			return
		}

		initialTx = txs[0]
	} else if req.Hash != "" {
		// Hash provided but no LT - search for transaction by hash in recent transactions
		txHashBytes, err := hex.DecodeString(req.Hash)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Success: false,
				Error:   "Invalid transaction hash: " + err.Error(),
			})
			return
		}

		master, err := h.api.CurrentMasterchainInfo(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to get master block: " + err.Error(),
			})
			return
		}

		account, err := h.api.GetAccount(ctx, master, addr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to get account: " + err.Error(),
			})
			return
		}

		if account == nil || !account.IsActive {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Success: false,
				Error:   "Account is not active",
			})
			return
		}

		// Search through recent transactions to find the one with matching hash
		currentLT := account.LastTxLT
		currentHash := account.LastTxHash
		found := false

		for i := 0; i < scanDepth && !found; i += 15 {
			txs, err := h.api.ListTransactions(ctx, addr, 15, currentLT, currentHash)
			if err != nil || len(txs) == 0 {
				break
			}

			for _, tx := range txs {
				if hex.EncodeToString(tx.Hash) == hex.EncodeToString(txHashBytes) {
					initialTx = tx
					found = true
					break
				}
			}

			if !found {
				lastTx := txs[len(txs)-1]
				currentLT = lastTx.LT
				currentHash = lastTx.Hash
			}
		}

		if !found {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Success: false,
				Error:   "Transaction not found in recent history",
			})
			return
		}
	} else {
		// Get latest transaction from account state
		master, err := h.api.CurrentMasterchainInfo(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to get master block: " + err.Error(),
			})
			return
		}

		account, err := h.api.GetAccount(ctx, master, addr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to get account: " + err.Error(),
			})
			return
		}

		if account == nil || !account.IsActive {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Success: false,
				Error:   "Account is not active",
			})
			return
		}

		txs, err := h.api.ListTransactions(ctx, addr, 1, account.LastTxLT, account.LastTxHash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Success: false,
				Error:   "Failed to fetch transaction: " + err.Error(),
			})
			return
		}

		if len(txs) == 0 {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Success: false,
				Error:   "No transactions found",
			})
			return
		}

		initialTx = txs[0]
	}

	// Trace transaction
	config := tracer.Config{
		ScanDepth: scanDepth,
		Testnet:   false,
		Verbose:   req.Verbose,
	}

	tracker, err := tracer.TraceTransaction(ctx, h.api, initialTx, config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to trace transaction: " + err.Error(),
		})
		return
	}

	// Build response
	accounts := make([]models.AccountInfo, 0, len(tracker.AccountStats))
	for _, stats := range tracker.AccountStats {
		balanceWithFees := new(big.Int).Set(stats.BalanceChange)
		balanceWithFees.Sub(balanceWithFees, stats.Fees)

		balanceTon := new(big.Float).SetInt(balanceWithFees)
		balanceTon.Quo(balanceTon, big.NewFloat(1e9))

		feesTon := new(big.Float).SetInt(stats.Fees)
		feesTon.Quo(feesTon, big.NewFloat(1e9))

		// Process jetton balances
		jettons := make([]models.JettonBalanceInfo, 0, len(stats.JettonBalances))
		for _, jettonBalance := range stats.JettonBalances {
			if jettonBalance.Amount.Cmp(big.NewInt(0)) == 0 {
				continue // Skip zero balances
			}

			// Skip pTON (Proxy TON) - it's wrapped TON already reflected in balance_change_ton
			if tracer.IsPTON(jettonBalance.JettonMaster) {
				continue
			}

			// Format jetton amount with correct decimals
			decimals := jettonBalance.Decimals
			if decimals == 0 {
				decimals = 9 // Default
			}

			divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
			quotient := new(big.Float).SetInt(jettonBalance.Amount)
			divisorFloat := new(big.Float).SetInt(divisor)
			result := new(big.Float).Quo(quotient, divisorFloat)

			jettons = append(jettons, models.JettonBalanceInfo{
				JettonWallet:  jettonBalance.JettonWallet,
				JettonMaster:  jettonBalance.JettonMaster,
				Symbol:        jettonBalance.Symbol,
				Decimals:      jettonBalance.Decimals,
				BalanceChange: result.Text('f', decimals),
			})
		}

		accounts = append(accounts, models.AccountInfo{
			Address:       stats.Address,
			BalanceChange: balanceTon.Text('f', 9),
			NetworkFees:   feesTon.Text('f', 9),
			Jettons:       jettons,
		})
	}

	c.JSON(http.StatusOK, models.TraceResponse{
		Success:           true,
		OriginalSender:    tracker.OriginalAddr,
		TotalTransactions: len(tracker.Transactions),
		Accounts:          accounts,
	})
}

// ResolveDNS godoc
// @Summary Resolve TON DNS domain
// @Description Resolve a .ton or .t.me domain to wallet address
// @Tags dns
// @Accept json
// @Produce json
// @Param request body models.ResolveRequest true "Resolve request"
// @Success 200 {object} models.ResolveResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/dns/resolve [post]
func (h *Handler) ResolveDNS(c *gin.Context) {
	var req models.ResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	ctx := context.Background()
	addr, err := tracer.ResolveAddress(ctx, h.api, req.Domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   "Failed to resolve domain: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.ResolveResponse{
		Success: true,
		Domain:  req.Domain,
		Address: addr.String(),
	})
}

// GetRecentTransactions godoc
// @Summary Get recent transactions
// @Description Get recent transactions for an address
// @Tags transactions
// @Accept json
// @Produce json
// @Param request body models.RecentTransactionsRequest true "Recent transactions request"
// @Success 200 {object} models.RecentTransactionsResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/transactions/recent [post]
func (h *Handler) GetRecentTransactions(c *gin.Context) {
	var req models.RecentTransactionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	count := req.Count
	if count == 0 || count > 100 {
		count = 10
	}

	ctx := context.Background()

	addr, err := tracer.ResolveAddress(ctx, h.api, req.Address)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   "Failed to resolve address: " + err.Error(),
		})
		return
	}

	master, err := h.api.CurrentMasterchainInfo(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to get master block: " + err.Error(),
		})
		return
	}

	account, err := h.api.GetAccount(ctx, master, addr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to get account: " + err.Error(),
		})
		return
	}

	if account == nil || !account.IsActive {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Success: false,
			Error:   "Account is not active",
		})
		return
	}

	txs, err := h.api.ListTransactions(ctx, addr, uint32(count), account.LastTxLT, account.LastTxHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to fetch transactions: " + err.Error(),
		})
		return
	}

	transactions := make([]models.TransactionItem, 0, len(txs))
	for _, tx := range txs {
		item := models.TransactionItem{
			Hash:      hex.EncodeToString(tx.Hash),
			LT:        tx.LT,
			Timestamp: int64(tx.Now),
		}

		if tx.IO.In != nil && tx.IO.In.MsgType == tlb.MsgTypeInternal {
			inMsg := tx.IO.In.AsInternal()
			tonAmount := new(big.Float).SetInt(inMsg.Amount.Nano())
			tonAmount.Quo(tonAmount, big.NewFloat(1e9))
			item.Amount = tonAmount.Text('f', 9)
		}

		transactions = append(transactions, item)
	}

	c.JSON(http.StatusOK, models.RecentTransactionsResponse{
		Success:      true,
		Address:      addr.String(),
		Transactions: transactions,
	})
}

// HealthCheck godoc
// @Summary Health check
// @Description Check if the service is healthy
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	})
}
