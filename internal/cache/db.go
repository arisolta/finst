package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB initializes the SQLite database at dbPath and runs migrations.
func InitDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		finstDir := filepath.Join(homeDir, ".finst")
		if err := os.MkdirAll(finstDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory %s: %w", finstDir, err)
		}
		dbPath = filepath.Join(finstDir, "cache.db")
	} else {
		dir := filepath.Dir(dbPath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db at %s: %w", dbPath, err)
	}

	// Optimize SQLite performance & concurrency
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA busy_timeout = 5000;
	`); err != nil {
		// Ignore pragma errors if in-memory
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS companies (
		ticker TEXT PRIMARY KEY,
		cik TEXT,
		name TEXT NOT NULL,
		exchange TEXT NOT NULL,
		sector TEXT,
		industry TEXT,
		currency TEXT NOT NULL,
		reporting_standard TEXT,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS financial_statements (
		ticker TEXT NOT NULL,
		period_type TEXT NOT NULL, -- 'ANNUAL' or 'QUARTERLY'
		fiscal_year INTEGER NOT NULL,
		fiscal_period TEXT NOT NULL, -- 'FY', 'Q1', 'Q2', 'Q3', 'Q4'
		period_end_date DATE NOT NULL,
		revenue REAL,
		cost_of_revenue REAL,
		gross_profit REAL,
		operating_income REAL,
		depreciation_amortization REAL,
		net_income REAL,
		diluted_shares REAL,
		adj_eps REAL,
		operating_cash_flow REAL,
		capex REAL,
		cash_and_equiv REAL,
		total_debt REAL,
		preferred_stock REAL,
		total_equity REAL,
		tax_expense REAL,
		pretax_income REAL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (ticker, period_type, fiscal_year, fiscal_period)
	);

	CREATE TABLE IF NOT EXISTS consensus_estimates (
		ticker TEXT NOT NULL,
		fiscal_year INTEGER NOT NULL,
		est_revenue REAL,
		est_ebitda REAL,
		est_ebit REAL,
		est_net_income REAL,
		est_eps REAL,
		est_capex REAL,
		est_cfo REAL,
		source TEXT NOT NULL, -- 'CONSENSUS' or 'MODEL_PROJECTION'
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (ticker, fiscal_year)
	);

	CREATE TABLE IF NOT EXISTS price_cache (
		ticker TEXT PRIMARY KEY,
		share_price REAL NOT NULL,
		shares_outstanding REAL NOT NULL,
		market_cap REAL NOT NULL,
		enterprise_value REAL NOT NULL,
		currency TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}
