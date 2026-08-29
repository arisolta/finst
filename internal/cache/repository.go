package cache

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"finst/internal/model"
)

const (
	TTLAnnualStatements    = 90 * 24 * time.Hour // 90 days
	TTLQuarterlyStatements = 14 * 24 * time.Hour // 14 days
	TTLConsensusEstimates  = 24 * time.Hour      // 24 hours
	TTLPriceValuation      = 15 * time.Minute    // 15 minutes
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// InvalidateTicker clears cached data for a ticker to force fresh fetch.
func (r *Repository) InvalidateTicker(ctx context.Context, ticker string) error {
	t := strings.ToUpper(ticker)
	queries := []string{
		"DELETE FROM companies WHERE ticker = ?",
		"DELETE FROM financial_statements WHERE ticker = ?",
		"DELETE FROM consensus_estimates WHERE ticker = ?",
		"DELETE FROM price_cache WHERE ticker = ?",
	}
	for _, q := range queries {
		if _, err := r.db.ExecContext(ctx, q, t); err != nil {
			return err
		}
	}
	return nil
}

// GetCompany retrieves cached company metadata.
func (r *Repository) GetCompany(ctx context.Context, ticker string) (*model.CompanyInfo, error) {
	t := strings.ToUpper(ticker)
	row := r.db.QueryRowContext(ctx, `
		SELECT ticker, cik, name, exchange, sector, industry, currency, reporting_standard, updated_at
		FROM companies WHERE ticker = ?
	`, t)

	var c model.CompanyInfo
	var cik, sector, industry, reportingStd sql.NullString
	if err := row.Scan(&c.Ticker, &cik, &c.Name, &c.Exchange, &sector, &industry, &c.Currency, &reportingStd, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.CIK = cik.String
	c.Sector = sector.String
	c.Industry = industry.String
	c.ReportingStandard = reportingStd.String
	return &c, nil
}

// SaveCompany inserts or updates company metadata.
func (r *Repository) SaveCompany(ctx context.Context, c *model.CompanyInfo) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO companies (ticker, cik, name, exchange, sector, industry, currency, reporting_standard, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ticker) DO UPDATE SET
			cik = excluded.cik,
			name = excluded.name,
			exchange = excluded.exchange,
			sector = excluded.sector,
			industry = excluded.industry,
			currency = excluded.currency,
			reporting_standard = excluded.reporting_standard,
			updated_at = excluded.updated_at
	`, strings.ToUpper(c.Ticker), c.CIK, c.Name, c.Exchange, c.Sector, c.Industry, c.Currency, c.ReportingStandard, time.Now())
	return err
}

// GetFinancialStatements retrieves financial statements and checks TTL validity.
func (r *Repository) GetFinancialStatements(ctx context.Context, ticker string) ([]model.FinancialStatement, bool, error) {
	t := strings.ToUpper(ticker)
	rows, err := r.db.QueryContext(ctx, `
		SELECT ticker, period_type, fiscal_year, fiscal_period, period_end_date,
		       revenue, cost_of_revenue, gross_profit, operating_income, depreciation_amortization,
		       net_income, diluted_shares, adj_eps, operating_cash_flow, capex,
		       cash_and_equiv, total_debt, preferred_stock, total_equity, tax_expense, pretax_income, historical_price, updated_at
		FROM financial_statements
		WHERE ticker = ?
		ORDER BY fiscal_year ASC, fiscal_period ASC
	`, t)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var statements []model.FinancialStatement
	isFresh := true
	now := time.Now()

	for rows.Next() {
		var st model.FinancialStatement
		var rev, cor, gp, oi, da, ni, ds, eps, cfo, capex, cash, debt, pref, eq, tax, pretax, hp sql.NullFloat64
		var endDate string

		if err := rows.Scan(
			&st.Ticker, &st.PeriodType, &st.FiscalYear, &st.FiscalPeriod, &endDate,
			&rev, &cor, &gp, &oi, &da, &ni, &ds, &eps, &cfo, &capex,
			&cash, &debt, &pref, &eq, &tax, &pretax, &hp, &st.UpdatedAt,
		); err != nil {
			return nil, false, err
		}

		st.PeriodEndDate = endDate
		st.Revenue = rev.Float64
		st.CostOfRevenue = cor.Float64
		st.GrossProfit = gp.Float64
		st.OperatingIncome = oi.Float64
		st.DepreciationAmortization = da.Float64
		st.NetIncome = ni.Float64
		st.DilutedShares = ds.Float64
		st.AdjEPS = eps.Float64
		st.OperatingCashFlow = cfo.Float64
		st.CapEx = capex.Float64
		st.CashAndEquiv = cash.Float64
		st.TotalDebt = debt.Float64
		st.PreferredStock = pref.Float64
		st.TotalEquity = eq.Float64
		st.TaxExpense = tax.Float64
		st.PretaxIncome = pretax.Float64
		st.HistoricalPrice = hp.Float64

		// Check TTL
		if st.PeriodType == model.PeriodAnnual {
			if now.Sub(st.UpdatedAt) > TTLAnnualStatements {
				isFresh = false
			}
		} else {
			if now.Sub(st.UpdatedAt) > TTLQuarterlyStatements {
				isFresh = false
			}
		}

		statements = append(statements, st)
	}

	if len(statements) == 0 {
		return nil, false, nil
	}

	return statements, isFresh, nil
}

// SaveFinancialStatements persists statements in batch.
func (r *Repository) SaveFinancialStatements(ctx context.Context, statements []model.FinancialStatement) error {
	if len(statements) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO financial_statements (
			ticker, period_type, fiscal_year, fiscal_period, period_end_date,
			revenue, cost_of_revenue, gross_profit, operating_income, depreciation_amortization,
			net_income, diluted_shares, adj_eps, operating_cash_flow, capex,
			cash_and_equiv, total_debt, preferred_stock, total_equity, tax_expense, pretax_income, historical_price, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ticker, period_type, fiscal_year, fiscal_period) DO UPDATE SET
			period_end_date = excluded.period_end_date,
			revenue = excluded.revenue,
			cost_of_revenue = excluded.cost_of_revenue,
			gross_profit = excluded.gross_profit,
			operating_income = excluded.operating_income,
			depreciation_amortization = excluded.depreciation_amortization,
			net_income = excluded.net_income,
			diluted_shares = excluded.diluted_shares,
			adj_eps = excluded.adj_eps,
			operating_cash_flow = excluded.operating_cash_flow,
			capex = excluded.capex,
			cash_and_equiv = excluded.cash_and_equiv,
			total_debt = excluded.total_debt,
			preferred_stock = excluded.preferred_stock,
			total_equity = excluded.total_equity,
			tax_expense = excluded.tax_expense,
			pretax_income = excluded.pretax_income,
			historical_price = excluded.historical_price,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, s := range statements {
		if s.PeriodEndDate == "" {
			s.PeriodEndDate = time.Now().Format("2006-01-02")
		}
		if _, err := stmt.ExecContext(ctx,
			strings.ToUpper(s.Ticker), s.PeriodType, s.FiscalYear, s.FiscalPeriod, s.PeriodEndDate,
			s.Revenue, s.CostOfRevenue, s.GrossProfit, s.OperatingIncome, s.DepreciationAmortization,
			s.NetIncome, s.DilutedShares, s.AdjEPS, s.OperatingCashFlow, s.CapEx,
			s.CashAndEquiv, s.TotalDebt, s.PreferredStock, s.TotalEquity, s.TaxExpense, s.PretaxIncome, s.HistoricalPrice, now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetConsensusEstimates retrieves forward consensus estimates.
func (r *Repository) GetConsensusEstimates(ctx context.Context, ticker string) ([]model.ConsensusEstimate, bool, error) {
	t := strings.ToUpper(ticker)
	rows, err := r.db.QueryContext(ctx, `
		SELECT ticker, fiscal_year, est_revenue, est_ebitda, est_ebit, est_net_income, est_eps, est_capex, est_cfo, source, updated_at
		FROM consensus_estimates
		WHERE ticker = ?
		ORDER BY fiscal_year ASC
	`, t)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var estimates []model.ConsensusEstimate
	isFresh := true
	now := time.Now()

	for rows.Next() {
		var est model.ConsensusEstimate
		var rev, ebitda, ebit, ni, eps, capex, cfo sql.NullFloat64

		if err := rows.Scan(
			&est.Ticker, &est.FiscalYear, &rev, &ebitda, &ebit, &ni, &eps, &capex, &cfo, &est.Source, &est.UpdatedAt,
		); err != nil {
			return nil, false, err
		}

		est.EstRevenue = rev.Float64
		est.EstEBITDA = ebitda.Float64
		est.EstEBIT = ebit.Float64
		est.EstNetIncome = ni.Float64
		est.EstEPS = eps.Float64
		est.EstCapEx = capex.Float64
		est.EstCFO = cfo.Float64

		if now.Sub(est.UpdatedAt) > TTLConsensusEstimates {
			isFresh = false
		}

		estimates = append(estimates, est)
	}

	if len(estimates) == 0 {
		return nil, false, nil
	}

	return estimates, isFresh, nil
}

// SaveConsensusEstimates persists forward estimates.
func (r *Repository) SaveConsensusEstimates(ctx context.Context, estimates []model.ConsensusEstimate) error {
	if len(estimates) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO consensus_estimates (
			ticker, fiscal_year, est_revenue, est_ebitda, est_ebit, est_net_income, est_eps, est_capex, est_cfo, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ticker, fiscal_year) DO UPDATE SET
			est_revenue = excluded.est_revenue,
			est_ebitda = excluded.est_ebitda,
			est_ebit = excluded.est_ebit,
			est_net_income = excluded.est_net_income,
			est_eps = excluded.est_eps,
			est_capex = excluded.est_capex,
			est_cfo = excluded.est_cfo,
			source = excluded.source,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, est := range estimates {
		if _, err := stmt.ExecContext(ctx,
			strings.ToUpper(est.Ticker), est.FiscalYear, est.EstRevenue, est.EstEBITDA, est.EstEBIT, est.EstNetIncome,
			est.EstEPS, est.EstCapEx, est.EstCFO, est.Source, now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetPriceValuation retrieves cached live price & valuation multiples.
func (r *Repository) GetPriceValuation(ctx context.Context, ticker string) (*model.PriceValuation, bool, error) {
	t := strings.ToUpper(ticker)
	row := r.db.QueryRowContext(ctx, `
		SELECT ticker, share_price, shares_outstanding, market_cap, enterprise_value, currency, updated_at
		FROM price_cache WHERE ticker = ?
	`, t)

	var pv model.PriceValuation
	if err := row.Scan(&pv.Ticker, &pv.SharePrice, &pv.SharesOutstanding, &pv.MarketCap, &pv.EnterpriseValue, &pv.Currency, &pv.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}

	isFresh := time.Since(pv.UpdatedAt) <= TTLPriceValuation
	return &pv, isFresh, nil
}

// SavePriceValuation persists live price & valuation metrics.
func (r *Repository) SavePriceValuation(ctx context.Context, pv *model.PriceValuation) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO price_cache (ticker, share_price, shares_outstanding, market_cap, enterprise_value, currency, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ticker) DO UPDATE SET
			share_price = excluded.share_price,
			shares_outstanding = excluded.shares_outstanding,
			market_cap = excluded.market_cap,
			enterprise_value = excluded.enterprise_value,
			currency = excluded.currency,
			updated_at = excluded.updated_at
	`, strings.ToUpper(pv.Ticker), pv.SharePrice, pv.SharesOutstanding, pv.MarketCap, pv.EnterpriseValue, pv.Currency, time.Now())
	return err
}
