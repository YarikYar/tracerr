package database

import (
	"time"
)

// TraceRequest represents a cached trace request
type TraceRequest struct {
	ID        int64     `db:"id"`
	Address   string    `db:"address"`
	Hash      *string   `db:"hash"`      // nullable
	LT        *uint64   `db:"lt"`        // nullable
	ScanDepth int       `db:"scan_depth"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// TraceResult represents a cached trace result
type TraceResult struct {
	ID                int64     `db:"id"`
	RequestID         int64     `db:"request_id"`
	Success           bool      `db:"success"`
	ErrorMessage      *string   `db:"error_message"` // nullable
	OriginalSender    string    `db:"original_sender"`
	TotalTransactions int       `db:"total_transactions"`
	CreatedAt         time.Time `db:"created_at"`
}

// AccountResult represents balance changes for an account in a trace
type AccountResult struct {
	ID               int64   `db:"id"`
	TraceResultID    int64   `db:"trace_result_id"`
	Address          string  `db:"address"`
	BalanceChangeTON string  `db:"balance_change_ton"` // stored as string to preserve precision
	NetworkFeesTON   string  `db:"network_fees_ton"`   // stored as string to preserve precision
	CreatedAt        time.Time `db:"created_at"`
}

// TraceCacheKey represents a unique identifier for a trace request
type TraceCacheKey struct {
	Address   string
	Hash      *string
	LT        *uint64
	ScanDepth int
}

// Equals checks if two cache keys are the same
func (k *TraceCacheKey) Equals(other *TraceCacheKey) bool {
	if k.Address != other.Address || k.ScanDepth != other.ScanDepth {
		return false
	}

	// Compare nullable Hash
	if (k.Hash == nil) != (other.Hash == nil) {
		return false
	}
	if k.Hash != nil && *k.Hash != *other.Hash {
		return false
	}

	// Compare nullable LT
	if (k.LT == nil) != (other.LT == nil) {
		return false
	}
	if k.LT != nil && *k.LT != *other.LT {
		return false
	}

	return true
}
