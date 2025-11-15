package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Repository handles database operations
type Repository struct {
	db *DB
}

// NewRepository creates a new repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// CachedTraceResult represents a complete cached trace result
type CachedTraceResult struct {
	RequestID         int64
	Success           bool
	ErrorMessage      *string
	OriginalSender    string
	TotalTransactions int
	Accounts          []CachedAccountResult
	CreatedAt         time.Time
}

// CachedAccountResult represents a cached account result
type CachedAccountResult struct {
	Address          string
	BalanceChangeTON string
	NetworkFeesTON   string
}

// FindCachedTrace looks for a cached trace result
func (r *Repository) FindCachedTrace(key *TraceCacheKey) (*CachedTraceResult, error) {
	// Find matching request
	query := `
		SELECT id, created_at
		FROM trace_requests
		WHERE address = $1
		  AND scan_depth = $2
		  AND (($3::VARCHAR IS NULL AND hash IS NULL) OR hash = $3)
		  AND (($4::BIGINT IS NULL AND lt IS NULL) OR lt = $4)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var requestID int64
	var createdAt time.Time

	var hashParam interface{}
	var ltParam interface{}

	if key.Hash != nil {
		hashParam = *key.Hash
	} else {
		hashParam = nil
	}

	if key.LT != nil {
		ltParam = *key.LT
	} else {
		ltParam = nil
	}

	err := r.db.QueryRow(query, key.Address, key.ScanDepth, hashParam, ltParam).Scan(&requestID, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil // No cached result
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find cached request: %w", err)
	}

	// Get trace result
	resultQuery := `
		SELECT success, error_message, original_sender, total_transactions
		FROM trace_results
		WHERE request_id = $1
		LIMIT 1
	`

	var result CachedTraceResult
	result.RequestID = requestID
	result.CreatedAt = createdAt

	err = r.db.QueryRow(resultQuery, requestID).Scan(
		&result.Success,
		&result.ErrorMessage,
		&result.OriginalSender,
		&result.TotalTransactions,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Request exists but no result yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trace result: %w", err)
	}

	// Get account results
	accountsQuery := `
		SELECT address, balance_change_ton, network_fees_ton
		FROM account_results
		WHERE trace_result_id = (
			SELECT id FROM trace_results WHERE request_id = $1 LIMIT 1
		)
		ORDER BY id
	`

	rows, err := r.db.Query(accountsQuery, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account results: %w", err)
	}
	defer rows.Close()

	result.Accounts = make([]CachedAccountResult, 0)
	for rows.Next() {
		var acc CachedAccountResult
		if err := rows.Scan(&acc.Address, &acc.BalanceChangeTON, &acc.NetworkFeesTON); err != nil {
			return nil, fmt.Errorf("failed to scan account result: %w", err)
		}
		result.Accounts = append(result.Accounts, acc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account results: %w", err)
	}

	return &result, nil
}

// SaveTraceResult saves a successful trace result to the cache
func (r *Repository) SaveTraceResult(
	key *TraceCacheKey,
	success bool,
	errorMessage *string,
	originalSender string,
	totalTransactions int,
	accounts []CachedAccountResult,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert trace request
	var requestID int64
	requestQuery := `
		INSERT INTO trace_requests (address, hash, lt, scan_depth)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	var hashParam interface{}
	var ltParam interface{}

	if key.Hash != nil {
		hashParam = *key.Hash
	} else {
		hashParam = nil
	}

	if key.LT != nil {
		ltParam = *key.LT
	} else {
		ltParam = nil
	}

	err = tx.QueryRow(requestQuery, key.Address, hashParam, ltParam, key.ScanDepth).Scan(&requestID)
	if err != nil {
		return fmt.Errorf("failed to insert trace request: %w", err)
	}

	// Insert trace result
	var resultID int64
	resultQuery := `
		INSERT INTO trace_results (request_id, success, error_message, original_sender, total_transactions)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	err = tx.QueryRow(resultQuery, requestID, success, errorMessage, originalSender, totalTransactions).Scan(&resultID)
	if err != nil {
		return fmt.Errorf("failed to insert trace result: %w", err)
	}

	// Insert account results
	if len(accounts) > 0 {
		accountQuery := `
			INSERT INTO account_results (trace_result_id, address, balance_change_ton, network_fees_ton)
			VALUES ($1, $2, $3, $4)
		`

		for _, acc := range accounts {
			_, err = tx.Exec(accountQuery, resultID, acc.Address, acc.BalanceChangeTON, acc.NetworkFeesTON)
			if err != nil {
				return fmt.Errorf("failed to insert account result: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetCacheStats returns statistics about the cache
func (r *Repository) GetCacheStats() (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(DISTINCT tr.id) as total_requests,
			COUNT(DISTINCT CASE WHEN tres.success = true THEN tr.id END) as successful_requests,
			COUNT(DISTINCT CASE WHEN tres.success = false THEN tr.id END) as failed_requests,
			COUNT(DISTINCT ar.address) as unique_addresses
		FROM trace_requests tr
		LEFT JOIN trace_results tres ON tr.id = tres.request_id
		LEFT JOIN account_results ar ON tres.id = ar.trace_result_id
	`

	var totalRequests, successfulRequests, failedRequests, uniqueAddresses int64

	err := r.db.QueryRow(query).Scan(&totalRequests, &successfulRequests, &failedRequests, &uniqueAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	stats := map[string]interface{}{
		"total_requests":      totalRequests,
		"successful_requests": successfulRequests,
		"failed_requests":     failedRequests,
		"unique_addresses":    uniqueAddresses,
		"cache_hit_rate":      0.0,
	}

	if totalRequests > 0 {
		stats["cache_hit_rate"] = float64(successfulRequests+failedRequests) / float64(totalRequests) * 100
	}

	return stats, nil
}

// ClearOldCache removes cache entries older than the specified duration
func (r *Repository) ClearOldCache(olderThan time.Duration) (int64, error) {
	query := `
		DELETE FROM trace_requests
		WHERE created_at < $1
	`

	threshold := time.Now().Add(-olderThan)
	result, err := r.db.Exec(query, threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to clear old cache: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}
