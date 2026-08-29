package model

import "time"

// Period constants
const (
	PeriodAnnual    = "ANNUAL"
	PeriodQuarterly = "QUARTERLY"

	FiscalPeriodFY = "FY"
	FiscalPeriodQ1 = "Q1"
	FiscalPeriodQ2 = "Q2"
	FiscalPeriodQ3 = "Q3"
	FiscalPeriodQ4 = "Q4"

	SourceConsensus = "CONSENSUS"
	SourceModel     = "MODEL_PROJECTION"

	PeriodTypeHistorical = "HISTORICAL"
	PeriodTypeLTM        = "LTM"
	PeriodTypeConsensus  = "CONSENSUS"
	PeriodTypeProjection = "PROJECTION"

	ViewStandard = "standard"
	ViewCompact  = "compact"

	ExportJSON = "json"
	ExportCSV  = "csv"
)

// CompanyInfo represents company metadata.
type CompanyInfo struct {
	Ticker            string    `json:"ticker"`
	CIK               string    `json:"cik,omitempty"`
	Name              string    `json:"name"`
	Exchange          string    `json:"exchange"`
	Sector            string    `json:"sector,omitempty"`
	Industry          string    `json:"industry,omitempty"`
	Currency          string    `json:"currency"`
	ReportingStandard string    `json:"reporting_standard,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// FinancialStatement represents a single fiscal period financial statement.
type FinancialStatement struct {
	Ticker                   string    `json:"ticker"`
	PeriodType               string    `json:"period_type"` // 'ANNUAL' or 'QUARTERLY'
	FiscalYear               int       `json:"fiscal_year"`
	FiscalPeriod             string    `json:"fiscal_period"` // 'FY', 'Q1', 'Q2', 'Q3', 'Q4'
	PeriodEndDate            string    `json:"period_end_date"`
	Revenue                  float64   `json:"revenue"`
	CostOfRevenue            float64   `json:"cost_of_revenue"`
	GrossProfit              float64   `json:"gross_profit"`
	OperatingIncome          float64   `json:"operating_income"` // EBIT
	DepreciationAmortization float64   `json:"depreciation_amortization"`
	NetIncome                float64   `json:"net_income"`
	DilutedShares            float64   `json:"diluted_shares"`
	AdjEPS                   float64   `json:"adj_eps"`
	OperatingCashFlow        float64   `json:"operating_cash_flow"` // CFO
	CapEx                    float64   `json:"capex"`
	CashAndEquiv             float64   `json:"cash_and_equiv"`
	TotalDebt                float64   `json:"total_debt"`
	PreferredStock           float64   `json:"preferred_stock"`
	TotalEquity              float64   `json:"total_equity"`
	TaxExpense               float64   `json:"tax_expense,omitempty"`
	PretaxIncome             float64   `json:"pretax_income,omitempty"`
	HistoricalPrice          float64   `json:"historical_price,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// ConsensusEstimate represents analyst consensus or forward projected estimates for a fiscal year.
type ConsensusEstimate struct {
	Ticker       string    `json:"ticker"`
	FiscalYear   int       `json:"fiscal_year"`
	EstRevenue   float64   `json:"est_revenue"`
	EstEBITDA    float64   `json:"est_ebitda"`
	EstEBIT      float64   `json:"est_ebit"`
	EstNetIncome float64   `json:"est_net_income"`
	EstEPS       float64   `json:"est_eps"`
	EstCapEx     float64   `json:"est_capex"`
	EstCFO       float64   `json:"est_cfo"`
	Source       string    `json:"source"` // 'CONSENSUS' or 'MODEL_PROJECTION'
	UpdatedAt    time.Time `json:"updated_at"`
}

// PriceValuation represents live market pricing and enterprise valuation cache.
type PriceValuation struct {
	Ticker            string    `json:"ticker"`
	SharePrice        float64   `json:"share_price"`
	SharesOutstanding float64   `json:"shares_outstanding"`
	MarketCap         float64   `json:"market_cap"`
	EnterpriseValue   float64   `json:"enterprise_value"`
	Currency          string    `json:"currency"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PeriodData holds calculated data for one of the 7 display columns.
type PeriodData struct {
	Label      string `json:"label"`       // e.g. "2023 Y", "LTM/Base", "2026E (Cons)", "2028E (Proj)"
	FiscalYear int    `json:"fiscal_year"` // 2023, 2024, etc.
	PeriodType string `json:"period_type"` // 'HISTORICAL', 'LTM', 'CONSENSUS', 'PROJECTION'
	IsForward  bool   `json:"is_forward"`

	// Capital Structure
	MarketCap         *float64 `json:"market_cap,omitempty"`
	CashAndEquiv      *float64 `json:"cash_and_equiv,omitempty"`
	PreferredAndOther *float64 `json:"preferred_and_other,omitempty"`
	TotalDebt         *float64 `json:"total_debt,omitempty"`
	EnterpriseValue   *float64 `json:"enterprise_value,omitempty"`

	// Operating Performance
	Revenue         float64  `json:"revenue"`
	YoYGrowthPct    *float64 `json:"yoy_growth_pct,omitempty"`
	GrossProfit     float64  `json:"gross_profit"`
	GrossMarginPct  *float64 `json:"gross_margin_pct,omitempty"`
	EBITDA          float64  `json:"ebitda"`
	EBITDAMarginPct *float64 `json:"ebitda_margin_pct,omitempty"`
	EBIT            float64  `json:"ebit"`
	EBITMarginPct   *float64 `json:"ebit_margin_pct,omitempty"`
	NetIncome       float64  `json:"net_income"`
	NetMarginPct    *float64 `json:"net_margin_pct,omitempty"`
	DilutedAdjEPS   float64  `json:"diluted_adj_eps"`
	EPSGrowthPct    *float64 `json:"eps_growth_pct,omitempty"`

	// Cash Flow Profile
	OperatingCashFlow        float64  `json:"operating_cash_flow"`
	DepreciationAmortization float64  `json:"depreciation_amortization"`
	CapEx                    float64  `json:"capex"`
	FreeCashFlow             float64  `json:"free_cash_flow"`
	FCFConversionPct         *float64 `json:"fcf_conversion_pct,omitempty"`

	// Returns & Profitability
	ROE  *float64 `json:"roe,omitempty"`
	ROIC *float64 `json:"roic,omitempty"`

	// Valuation Multiples
	PE       *float64 `json:"pe,omitempty"`
	PB       *float64 `json:"pb,omitempty"`
	PFCF     *float64 `json:"pfcf,omitempty"`
	EVSales  *float64 `json:"ev_sales,omitempty"`
	EVEBITDA *float64 `json:"ev_ebitda,omitempty"`
	EVEBIT   *float64 `json:"ev_ebit,omitempty"`
}

// FinancialDataset represents the full normalized dataset ready for UI or Export.
type FinancialDataset struct {
	Company         CompanyInfo    `json:"company"`
	Price           PriceValuation `json:"price"`
	DisplayCurrency string         `json:"display_currency"`
	ScaleUnit       string         `json:"scale_unit"` // e.g. "in Millions"
	Periods         []PeriodData   `json:"periods"`    // 7 periods
}
