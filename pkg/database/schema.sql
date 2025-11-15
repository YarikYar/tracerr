-- Database schema for TON Transaction Tracer cache

-- Enable UUID extension for potential future use
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Trace requests table - stores unique trace requests
CREATE TABLE IF NOT EXISTS trace_requests (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(255) NOT NULL,
    hash VARCHAR(128),  -- nullable, hex-encoded transaction hash
    lt BIGINT,          -- nullable, logical time
    scan_depth INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index on address for faster lookups
CREATE INDEX IF NOT EXISTS idx_trace_requests_address ON trace_requests(address);

-- Create composite index for cache key lookup
CREATE INDEX IF NOT EXISTS idx_trace_requests_cache_key
    ON trace_requests(address, hash, lt, scan_depth);

-- Trace results table - stores the results of trace operations
CREATE TABLE IF NOT EXISTS trace_results (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES trace_requests(id) ON DELETE CASCADE,
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,  -- nullable, only set if success = false
    original_sender VARCHAR(255) NOT NULL,
    total_transactions INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index on request_id for faster joins
CREATE INDEX IF NOT EXISTS idx_trace_results_request_id ON trace_results(request_id);

-- Account results table - stores per-account balance changes
CREATE TABLE IF NOT EXISTS account_results (
    id BIGSERIAL PRIMARY KEY,
    trace_result_id BIGINT NOT NULL REFERENCES trace_results(id) ON DELETE CASCADE,
    address VARCHAR(255) NOT NULL,
    balance_change_ton DECIMAL(30, 9) NOT NULL,  -- high precision for TON amounts
    network_fees_ton DECIMAL(30, 9) NOT NULL,    -- high precision for TON amounts
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index on trace_result_id for faster joins
CREATE INDEX IF NOT EXISTS idx_account_results_trace_result_id
    ON account_results(trace_result_id);

-- Create index on address for analytics
CREATE INDEX IF NOT EXISTS idx_account_results_address ON account_results(address);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at
CREATE TRIGGER update_trace_requests_updated_at
    BEFORE UPDATE ON trace_requests
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- View for easy cache lookups with results
CREATE OR REPLACE VIEW trace_cache AS
SELECT
    tr.id as request_id,
    tr.address,
    tr.hash,
    tr.lt,
    tr.scan_depth,
    tres.id as result_id,
    tres.success,
    tres.error_message,
    tres.original_sender,
    tres.total_transactions,
    tres.created_at as result_created_at,
    tr.created_at as request_created_at
FROM trace_requests tr
LEFT JOIN trace_results tres ON tr.id = tres.request_id
ORDER BY tr.created_at DESC;

-- View for cache statistics
CREATE OR REPLACE VIEW cache_stats AS
SELECT
    COUNT(DISTINCT tr.id) as total_requests,
    COUNT(DISTINCT CASE WHEN tres.success = true THEN tr.id END) as successful_requests,
    COUNT(DISTINCT CASE WHEN tres.success = false THEN tr.id END) as failed_requests,
    COUNT(DISTINCT ar.address) as unique_addresses_tracked,
    COUNT(ar.id) as total_account_results,
    MAX(tr.created_at) as last_request_time
FROM trace_requests tr
LEFT JOIN trace_results tres ON tr.id = tres.request_id
LEFT JOIN account_results ar ON tres.id = ar.trace_result_id;
