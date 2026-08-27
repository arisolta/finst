package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"finst/internal/model"
)

type EdgarService struct {
	client    *Client
	cikMap    map[string]cikEntry
	cikLock   sync.RWMutex
	mapLoaded bool
}

type cikEntry struct {
	CIK    string `json:"cik_str"`
	Ticker string `json:"ticker"`
	Title  string `json:"title"`
}

func NewEdgarService(client *Client) *EdgarService {
	return &EdgarService{
		client: client,
		cikMap: make(map[string]cikEntry),
	}
}

// EnsureCIKMap loads the ticker-to-CIK mapping from SEC if not already loaded.
func (s *EdgarService) EnsureCIKMap(ctx context.Context) error {
	s.cikLock.Lock()
	defer s.cikLock.Unlock()

	if s.mapLoaded && len(s.cikMap) > 0 {
		return nil
	}

	url := "https://www.sec.gov/files/company_tickers.json"
	opts := &RequestOptions{
		Headers: map[string]string{
			"User-Agent": SECUserAgent,
			"Accept":     "application/json",
		},
		Timeout: 20 * time.Second,
		Retries: 3,
	}

	data, err := s.client.Get(ctx, url, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch SEC ticker directory: %w", err)
	}

	var rawMap map[string]struct {
		CIK    int64  `json:"cik_str"`
		Ticker string `json:"ticker"`
		Title  string `json:"title"`
	}

	if err := json.Unmarshal(data, &rawMap); err != nil {
		return fmt.Errorf("failed to parse SEC ticker json: %w", err)
	}

	for _, item := range rawMap {
		paddedCIK := fmt.Sprintf("%010d", item.CIK)
		upperTicker := strings.ToUpper(strings.TrimSpace(item.Ticker))
		s.cikMap[upperTicker] = cikEntry{
			CIK:    paddedCIK,
			Ticker: upperTicker,
			Title:  item.Title,
		}
	}

	s.mapLoaded = true
	return nil
}

// ResolveTicker maps a ticker symbol to its zero-padded 10-digit CIK and Company Title.
func (s *EdgarService) ResolveTicker(ctx context.Context, ticker string) (string, string, error) {
	upper := strings.ToUpper(strings.TrimSpace(ticker))
	if err := s.EnsureCIKMap(ctx); err != nil {
		return "", "", err
	}

	s.cikLock.RLock()
	defer s.cikLock.RUnlock()

	entry, ok := s.cikMap[upper]
	if !ok {
		return "", "", fmt.Errorf("ticker %s not found in SEC EDGAR directory", ticker)
	}
	return entry.CIK, entry.Title, nil
}

// SEC Facts Data Structure
type SECCompanyFacts struct {
	CIK        int    `json:"cik"`
	EntityName string `json:"entityName"`
	Facts      struct {
		Dei    map[string]SECFactConcept `json:"dei"`
		USGAAP map[string]SECFactConcept `json:"us-gaap"`
		IFRS   map[string]SECFactConcept `json:"ifrs-full"`
	} `json:"facts"`
}

type SECFactConcept struct {
	Label       string                   `json:"label"`
	Description string                   `json:"description"`
	Units       map[string][]SECFactUnit `json:"units"`
}

type SECFactUnit struct {
	End   string  `json:"end"`
	Start string  `json:"start,omitempty"`
	Val   float64 `json:"val"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
	Frame string  `json:"frame,omitempty"`
}

// FetchCompanyFacts retrieves XBRL JSON facts from SEC EDGAR.
func (s *EdgarService) FetchCompanyFacts(ctx context.Context, paddedCIK string) (*SECCompanyFacts, error) {
	url := fmt.Sprintf("https://data.sec.gov/api/xbrl/companyfacts/CIK%s.json", paddedCIK)
	opts := &RequestOptions{
		Headers: map[string]string{
			"User-Agent": SECUserAgent,
			"Accept":     "application/json",
		},
		Timeout: 20 * time.Second,
		Retries: 3,
	}

	data, err := s.client.Get(ctx, url, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SEC facts for CIK %s: %w", paddedCIK, err)
	}

	var facts SECCompanyFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SEC facts: %w", err)
	}

	return &facts, nil
}

// ExtractStatements processes SEC XBRL facts into structured FinancialStatement records.
func (s *EdgarService) ExtractStatements(facts *SECCompanyFacts, ticker string) ([]model.FinancialStatement, error) {
	if facts == nil {
		return nil, fmt.Errorf("empty SEC facts")
	}

	gaap := facts.Facts.USGAAP
	if len(gaap) == 0 {
		gaap = facts.Facts.IFRS
		if len(gaap) == 0 {
			return nil, fmt.Errorf("no us-gaap or ifrs-full facts found in SEC filing")
		}
	}

	type StKey struct {
		PeriodType   string // ANNUAL or QUARTERLY
		FiscalYear   int
		FiscalPeriod string // FY, Q1, Q2, Q3, Q4
	}

	statementMap := make(map[StKey]*model.FinancialStatement)
	filedMap := make(map[StKey]string)

	getOrCreate := func(pType string, fy int, fp, endDate, filed string) *model.FinancialStatement {
		k := StKey{PeriodType: pType, FiscalYear: fy, FiscalPeriod: fp}
		if st, ok := statementMap[k]; ok {
			if filed > filedMap[k] && endDate != "" {
				st.PeriodEndDate = endDate
				filedMap[k] = filed
			}
			return st
		}
		st := &model.FinancialStatement{
			Ticker:        ticker,
			PeriodType:    pType,
			FiscalYear:    fy,
			FiscalPeriod:  fp,
			PeriodEndDate: endDate,
			UpdatedAt:     time.Now(),
		}
		statementMap[k] = st
		filedMap[k] = filed
		return st
	}

	// Concept candidates in order of priority (supporting US-GAAP and IFRS-full)
	conceptRev := []string{
		"RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "Revenue",
		"RevenueFromContractsWithCustomers", "SalesRevenueNet", "TotalRevenuesAndOtherIncome",
		"SalesRevenueServicesNet", "SalesRevenueGoodsNet",
	}
	conceptCost := []string{
		"CostOfGoodsAndServicesSold", "CostOfSales", "CostOfRevenue", "CostOfServices",
		"CostOfGoodsSold", "CostOfPurchasedGoodsAndServices",
	}
	conceptGross := []string{"GrossProfit"}
	conceptEBIT := []string{
		"OperatingIncomeLoss", "OperatingProfitLoss", "ProfitLossFromOperatingActivities",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesMinorityInterestAndIncomeLossFromEquityMethodInvestments",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxes", "OperatingIncome",
	}
	conceptDA := []string{
		"DepreciationDepletionAndAmortization", "DepreciationAndAmortization",
		"DepreciationAndAmortisationExpense", "DepreciationAmortisationAndImpairmentLossesExcludingImpairmentLossesReversed",
		"Depreciation", "AmortizationOfIntangibleAssets", "CapitalLeasesIncomeStatementAmortizationExpense",
	}
	conceptNet := []string{"NetIncomeLoss", "ProfitLoss", "ProfitLossAttributableToOwnersOfParent"}
	conceptCFO := []string{"NetCashProvidedByUsedInOperatingActivities", "CashFlowsFromUsedInOperatingActivities"}
	conceptCapEx := []string{
		"PaymentsToAcquirePropertyPlantAndEquipment", "PurchaseOfPropertyPlantAndEquipmentClassifiedAsInvestingActivities",
		"PaymentsToAcquireProductiveAssets", "PaymentsToAcquirePropertyPlantAndEquipmentAndIntangibleAssets",
	}
	conceptCash := []string{
		"CashAndCashEquivalentsAtCarryingValue", "CashAndCashEquivalents",
		"CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalents",
	}
	conceptEquity := []string{
		"StockholdersEquity", "Equity", "EquityAttributableToOwnersOfParent",
		"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest",
	}
	conceptPreferred := []string{"PreferredStockValue", "PreferredStockValueOutstanding"}
	conceptDebtLT := []string{
		"LongTermDebtAndCapitalLeaseObligations", "LongTermDebtNoncurrent", "LongTermDebt",
		"NoncurrentBorrowings", "Borrowings", "DebtAndCapitalLeaseObligations",
	}
	conceptDebtST := []string{
		"LongTermDebtAndCapitalLeaseObligationsCurrent", "ShortTermBorrowings", "CurrentBorrowings",
		"LongTermDebtCurrent", "DebtCurrent",
	}
	conceptEPS := []string{"EarningsPerShareDiluted", "DilutedEarningsLossPerShare", "BasicAndDilutedEarningsLossPerShare", "EarningsPerShareBasic"}
	conceptShares := []string{"WeightedAverageNumberOfDilutedSharesOutstanding", "WeightedAverageShares", "CommonStockSharesOutstanding"}

	determineYear := func(u SECFactUnit) int {
		if u.End != "" {
			if t, err := time.Parse("2006-01-02", u.End); err == nil {
				return t.Year()
			}
		}
		return u.FY
	}

	type YTDKey struct {
		Concept    string
		FiscalYear int
	}
	ytdMap := make(map[YTDKey]map[string]float64) // Concept+FY -> "Q1", "Q2", "Q3", "FY" -> val

	setYTD := func(concept string, fy int, period string, val float64) {
		k := YTDKey{Concept: concept, FiscalYear: fy}
		if ytdMap[k] == nil {
			ytdMap[k] = make(map[string]float64)
		}
		if existing, ok := ytdMap[k][period]; !ok || math.Abs(val) > math.Abs(existing) {
			ytdMap[k][period] = val
		}
	}

	// Helper to extract duration entries
	processDuration := func(conceptNames []string, isCashFlow bool, setter func(st *model.FinancialStatement, val float64)) {
		for _, name := range conceptNames {
			concept, ok := gaap[name]
			if !ok {
				continue
			}
			for _, units := range concept.Units {
				for _, u := range units {
					fy := determineYear(u)
					if fy == 0 {
						continue
					}

					var days float64 = 365
					if u.Start != "" && u.End != "" {
						stDate, err1 := time.Parse("2006-01-02", u.Start)
						enDate, err2 := time.Parse("2006-01-02", u.End)
						if err1 == nil && err2 == nil {
							days = enDate.Sub(stDate).Hours() / 24
						}
					}

					isAnnual := (u.FP == "FY" || strings.HasSuffix(u.Frame, "FY") || days > 300)
					isQuarter := (days >= 70 && days <= 110)
					is6M := (days >= 160 && days <= 200)
					is9M := (days >= 250 && days <= 290)

					if isAnnual {
						st := getOrCreate(model.PeriodAnnual, fy, model.FiscalPeriodFY, u.End, u.Filed)
						setter(st, u.Val)
						if isCashFlow {
							setYTD(name, fy, "FY", u.Val)
						}
					} else if isQuarter {
						fp := u.FP
						if fp == "" || fp == "FY" {
							if u.End != "" {
								if t, err := time.Parse("2006-01-02", u.End); err == nil {
									month := int(t.Month())
									if month <= 3 {
										fp = "Q1"
									} else if month <= 6 {
										fp = "Q2"
									} else if month <= 9 {
										fp = "Q3"
									} else {
										fp = "Q4"
									}
								}
							}
						}
						st := getOrCreate(model.PeriodQuarterly, fy, fp, u.End, u.Filed)
						setter(st, u.Val)
						if isCashFlow && fp == "Q1" {
							setYTD(name, fy, "Q1", u.Val)
						}
					} else if isCashFlow {
						if is6M {
							setYTD(name, fy, "Q2", u.Val)
						} else if is9M {
							setYTD(name, fy, "Q3", u.Val)
						}
					}
				}
			}
		}
	}

	// Helper to extract instant entries (balance sheet)
	processInstant := func(conceptNames []string, setter func(st *model.FinancialStatement, val float64)) {
		for _, name := range conceptNames {
			concept, ok := gaap[name]
			if !ok {
				continue
			}
			for _, units := range concept.Units {
				for _, u := range units {
					fy := determineYear(u)
					if fy == 0 {
						continue
					}
					isAnnual := (u.FP == "FY" || strings.HasSuffix(u.Frame, "FY"))
					if isAnnual {
						st := getOrCreate(model.PeriodAnnual, fy, model.FiscalPeriodFY, u.End, u.Filed)
						setter(st, u.Val)
					} else {
						st := getOrCreate(model.PeriodQuarterly, fy, u.FP, u.End, u.Filed)
						setter(st, u.Val)
					}
				}
			}
		}
	}

	processDuration(conceptRev, false, func(st *model.FinancialStatement, val float64) {
		if st.Revenue == 0 || val > st.Revenue {
			st.Revenue = val
		}
	})
	processDuration(conceptCost, false, func(st *model.FinancialStatement, val float64) {
		if st.CostOfRevenue == 0 || val > st.CostOfRevenue {
			st.CostOfRevenue = val
		}
	})
	processDuration(conceptGross, false, func(st *model.FinancialStatement, val float64) {
		if st.GrossProfit == 0 || val > st.GrossProfit {
			st.GrossProfit = val
		}
	})
	processDuration(conceptEBIT, false, func(st *model.FinancialStatement, val float64) {
		if st.OperatingIncome == 0 {
			st.OperatingIncome = val
		}
	})
	processDuration(conceptDA, true, func(st *model.FinancialStatement, val float64) {
		if st.DepreciationAmortization == 0 || val > st.DepreciationAmortization {
			st.DepreciationAmortization = val
		}
	})
	processDuration(conceptNet, false, func(st *model.FinancialStatement, val float64) {
		if st.NetIncome == 0 {
			st.NetIncome = val
		}
	})
	processDuration(conceptCFO, true, func(st *model.FinancialStatement, val float64) {
		if st.OperatingCashFlow == 0 {
			st.OperatingCashFlow = val
		}
	})
	processDuration(conceptCapEx, true, func(st *model.FinancialStatement, val float64) {
		if st.CapEx == 0 || val > st.CapEx {
			st.CapEx = val
		}
	})
	processDuration(conceptEPS, false, func(st *model.FinancialStatement, val float64) {
		if st.AdjEPS == 0 {
			st.AdjEPS = val
		}
	})
	processDuration(conceptShares, false, func(st *model.FinancialStatement, val float64) {
		if st.DilutedShares == 0 || val > st.DilutedShares {
			st.DilutedShares = val
		}
	})

	// De-accumulate YTD cash flows into discrete quarterly cash flows
	for _, cNames := range [][]string{conceptCFO, conceptCapEx, conceptDA} {
		for _, name := range cNames {
			for k, pMap := range ytdMap {
				if k.Concept != name {
					continue
				}
				fy := k.FiscalYear
				q1Val := pMap["Q1"]
				q2YTD := pMap["Q2"]
				q3YTD := pMap["Q3"]
				fyVal := pMap["FY"]

				setter := func(fp string, val float64) {
					sk := StKey{PeriodType: model.PeriodQuarterly, FiscalYear: fy, FiscalPeriod: fp}
					if st, ok := statementMap[sk]; ok {
						switch name {
						case "NetCashProvidedByUsedInOperatingActivities", "CashFlowsFromUsedInOperatingActivities":
							if st.OperatingCashFlow == 0 {
								st.OperatingCashFlow = val
							}
						case "PaymentsToAcquirePropertyPlantAndEquipment", "PurchaseOfPropertyPlantAndEquipmentClassifiedAsInvestingActivities",
							"PaymentsToAcquireProductiveAssets", "PaymentsToAcquirePropertyPlantAndEquipmentAndIntangibleAssets":
							if st.CapEx == 0 {
								st.CapEx = val
							}
						case "DepreciationDepletionAndAmortization", "DepreciationAndAmortization",
							"DepreciationAndAmortisationExpense", "Depreciation":
							if st.DepreciationAmortization == 0 {
								st.DepreciationAmortization = val
							}
						}
					}
				}

				if q1Val != 0 {
					setter("Q1", q1Val)
				}
				if q2YTD != 0 {
					setter("Q2", q2YTD-q1Val)
				}
				if q3YTD != 0 && q2YTD != 0 {
					setter("Q3", q3YTD-q2YTD)
				}
				if fyVal != 0 && q3YTD != 0 {
					setter("Q4", fyVal-q3YTD)
				}
			}
		}
	}

	processInstant(conceptCash, func(st *model.FinancialStatement, val float64) {
		if st.CashAndEquiv == 0 || val > st.CashAndEquiv {
			st.CashAndEquiv = val
		}
	})
	processInstant(conceptEquity, func(st *model.FinancialStatement, val float64) {
		if st.TotalEquity == 0 || val > st.TotalEquity {
			st.TotalEquity = val
		}
	})
	processInstant(conceptPreferred, func(st *model.FinancialStatement, val float64) {
		if st.PreferredStock == 0 {
			st.PreferredStock = val
		}
	})
	ltDebtMap := make(map[StKey]float64)
	stDebtMap := make(map[StKey]float64)

	processInstant(conceptDebtLT, func(st *model.FinancialStatement, val float64) {
		k := StKey{PeriodType: st.PeriodType, FiscalYear: st.FiscalYear, FiscalPeriod: st.FiscalPeriod}
		if existing, ok := ltDebtMap[k]; !ok || val > existing {
			ltDebtMap[k] = val
		}
	})
	processInstant(conceptDebtST, func(st *model.FinancialStatement, val float64) {
		k := StKey{PeriodType: st.PeriodType, FiscalYear: st.FiscalYear, FiscalPeriod: st.FiscalPeriod}
		if existing, ok := stDebtMap[k]; !ok || val > existing {
			stDebtMap[k] = val
		}
	})

	for k, st := range statementMap {
		st.TotalDebt = ltDebtMap[k] + stDebtMap[k]
	}

	// Post-processing
	var results []model.FinancialStatement
	for _, st := range statementMap {
		if st.Revenue == 0 && st.NetIncome == 0 && st.TotalEquity == 0 {
			continue
		}
		if st.GrossProfit == 0 && st.Revenue != 0 && st.CostOfRevenue != 0 {
			st.GrossProfit = st.Revenue - st.CostOfRevenue
		}
		if st.AdjEPS == 0 && st.DilutedShares > 0 && st.NetIncome != 0 {
			st.AdjEPS = st.NetIncome / st.DilutedShares
		}
		results = append(results, *st)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].FiscalYear != results[j].FiscalYear {
			return results[i].FiscalYear < results[j].FiscalYear
		}
		return results[i].FiscalPeriod < results[j].FiscalPeriod
	})

	return results, nil
}
