package handlers

import (
	"context"
	"encoding/hex"
	"log"
	"math/big"
	"net/http"

	"ton-tracer/api/models"
	"ton-tracer/pkg/database"
	"ton-tracer/pkg/tracer"

	"github.com/gin-gonic/gin"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type Handler struct {
	client *liteclient.ConnectionPool
	api    ton.APIClientWrapped
	repo   *database.Repository
}

func NewHandler(testnet bool, repo *database.Repository) (*Handler, error) {
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
		repo:   repo,
	}, nil
}

// TraceTransaction godoc
// @Summary Trace a TON transaction
// @Description Trace a transaction chain and return balance changes for all involved accounts. Uses database cache for performance.
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

	// Create cache key
	cacheKey := &database.TraceCacheKey{
		Address:   addr.String(),
		ScanDepth: scanDepth,
	}

	if req.Hash != "" {
		cacheKey.Hash = &req.Hash
	}

	if req.LT != 0 {
		cacheKey.LT = &req.LT
	}

	// Check cache first
	if h.repo != nil {
		cached, err := h.repo.FindCachedTrace(cacheKey)
		if err != nil {
			log.Printf("Cache lookup error: %v", err)
			// Continue with normal flow on cache error
		} else if cached != nil {
			log.Printf("Cache hit for address %s", addr.String())

			// Return cached result
			accounts := make([]models.AccountInfo, 0, len(cached.Accounts))
			for _, acc := range cached.Accounts {
				accounts = append(accounts, models.AccountInfo{
					Address:       acc.Address,
					BalanceChange: acc.BalanceChangeTON,
					NetworkFees:   acc.NetworkFeesTON,
				})
			}

			c.JSON(http.StatusOK, models.TraceResponse{
				Success:           cached.Success,
				OriginalSender:    cached.OriginalSender,
				TotalTransactions: cached.TotalTransactions,
				Accounts:          accounts,
				Cached:            true,
			})
			return
		}
	}

	log.Printf("Cache miss for address %s, performing blockchain scan", addr.String())

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

		// Update cache key with actual hash and LT
		if len(initialTx.Hash) > 0 {
			hashStr := hex.EncodeToString(initialTx.Hash)
			cacheKey.Hash = &hashStr
		}
		cacheKey.LT = &initialTx.LT
	}

	// Trace transaction
	config := tracer.Config{
		ScanDepth: scanDepth,
		Testnet:   false,
		Verbose:   false,
	}

	tracker, err := tracer.TraceTransaction(ctx, h.api, initialTx, config)
	if err != nil {
		// Save error to cache
		if h.repo != nil {
			errMsg := err.Error()
			saveErr := h.repo.SaveTraceResult(cacheKey, false, &errMsg, "", 0, nil)
			if saveErr != nil {
				log.Printf("Failed to cache error: %v", saveErr)
			}
		}

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to trace transaction: " + err.Error(),
		})
		return
	}

	// Build response
	accounts := make([]models.AccountInfo, 0, len(tracker.AccountStats))
	cachedAccounts := make([]database.CachedAccountResult, 0, len(tracker.AccountStats))

	for _, stats := range tracker.AccountStats {
		balanceWithFees := new(big.Int).Set(stats.BalanceChange)
		balanceWithFees.Sub(balanceWithFees, stats.Fees)

		balanceTon := new(big.Float).SetInt(balanceWithFees)
		balanceTon.Quo(balanceTon, big.NewFloat(1e9))

		feesTon := new(big.Float).SetInt(stats.Fees)
		feesTon.Quo(feesTon, big.NewFloat(1e9))

		balanceStr := balanceTon.Text('f', 9)
		feesStr := feesTon.Text('f', 9)

		accounts = append(accounts, models.AccountInfo{
			Address:       stats.Address,
			BalanceChange: balanceStr,
			NetworkFees:   feesStr,
		})

		cachedAccounts = append(cachedAccounts, database.CachedAccountResult{
			Address:          stats.Address,
			BalanceChangeTON: balanceStr,
			NetworkFeesTON:   feesStr,
		})
	}

	// Save to cache
	if h.repo != nil {
		err := h.repo.SaveTraceResult(
			cacheKey,
			true,
			nil,
			tracker.OriginalAddr,
			len(tracker.Transactions),
			cachedAccounts,
		)
		if err != nil {
			log.Printf("Failed to cache result: %v", err)
		} else {
			log.Printf("Saved trace result to cache for address %s", addr.String())
		}
	}

	c.JSON(http.StatusOK, models.TraceResponse{
		Success:           true,
		OriginalSender:    tracker.OriginalAddr,
		TotalTransactions: len(tracker.Transactions),
		Accounts:          accounts,
		Cached:            false,
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

// GetCacheStats godoc
// @Summary Get cache statistics
// @Description Get statistics about the database cache
// @Tags cache
// @Produce json
// @Success 200 {object} models.CacheStatsResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/cache/stats [get]
func (h *Handler) GetCacheStats(c *gin.Context) {
	if h.repo == nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Success: false,
			Error:   "Database not configured",
		})
		return
	}

	stats, err := h.repo.GetCacheStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Success: false,
			Error:   "Failed to get cache stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.CacheStatsResponse{
		Success: true,
		Stats:   stats,
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
		Version: "1.0.0-with-db",
	})
}
