package models

// TraceRequest represents a request to trace a transaction
type TraceRequest struct {
	Address   string `json:"address" binding:"required" example:"EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"`
	Hash      string `json:"hash,omitempty" example:"0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229"`
	LT        uint64 `json:"lt,omitempty" example:"63633416000001"`
	ScanDepth int    `json:"scan_depth,omitempty" example:"1000"`
}

// JettonBalanceInfo represents a jetton balance change for an account
type JettonBalanceInfo struct {
	JettonWallet  string `json:"jetton_wallet" example:"EQCxE6mUtQJKFnGfaROTKOt1lZbDiiX1kCixRv7Nw2Id_sDs"`
	JettonMaster  string `json:"jetton_master" example:"EQCxE6mUtQJKFnGfaROTKOt1lZbDiiX1kCixRv7Nw2Id_sDs"`
	Symbol        string `json:"symbol" example:"USDT"`
	Decimals      int    `json:"decimals" example:"6"`
	BalanceChange string `json:"balance_change" example:"100.000000"`
}

// AccountInfo represents statistics for a single account
type AccountInfo struct {
	Address       string              `json:"address" example:"EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"`
	BalanceChange string              `json:"balance_change_ton" example:"-0.115324419"`
	NetworkFees   string              `json:"network_fees_ton" example:"0.005692355"`
	Jettons       []JettonBalanceInfo `json:"jettons,omitempty"`
}

// TraceResponse represents the response from a trace operation
type TraceResponse struct {
	Success           bool          `json:"success" example:"true"`
	OriginalSender    string        `json:"original_sender" example:"EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"`
	TotalTransactions int           `json:"total_transactions" example:"9"`
	Accounts          []AccountInfo `json:"accounts"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Error   string `json:"error" example:"Invalid address format"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string `json:"status" example:"healthy"`
	Version string `json:"version" example:"1.0.0"`
}

// ResolveRequest represents a DNS resolution request
type ResolveRequest struct {
	Domain string `json:"domain" binding:"required" example:"foundation.ton"`
}

// ResolveResponse represents a DNS resolution response
type ResolveResponse struct {
	Success bool   `json:"success" example:"true"`
	Domain  string `json:"domain" example:"foundation.ton"`
	Address string `json:"address" example:"EQAU_6rW5f5P3eZCANkI2zCgIJl6PqDLuHVs2LmNaLfSXwIa"`
}

// RecentTransactionsRequest represents a request for recent transactions
type RecentTransactionsRequest struct {
	Address string `json:"address" binding:"required" example:"EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"`
	Count   int    `json:"count,omitempty" example:"10"`
}

// TransactionItem represents a single transaction in the list
type TransactionItem struct {
	Hash      string `json:"hash" example:"0a1073ddaae99eed5a6de55cbd60aee18b1fc3deeee59414024679bf00172229"`
	LT        uint64 `json:"lt" example:"63633416000001"`
	Timestamp int64  `json:"timestamp" example:"1699876543"`
	Amount    string `json:"amount,omitempty" example:"0.05"`
}

// RecentTransactionsResponse represents the response for recent transactions
type RecentTransactionsResponse struct {
	Success      bool              `json:"success" example:"true"`
	Address      string            `json:"address" example:"EQACkbxaojFxCOihpgl-Xh5yJdnR1lgBN-QnRz9xrZVEpZdf"`
	Transactions []TransactionItem `json:"transactions"`
}
