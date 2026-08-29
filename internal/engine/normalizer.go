package engine

import (
	"context"
	"math"
	"sort"
	"time"

	"finst/internal/api"
	"finst/internal/model"
)

type StatementNormalizer struct {
	fxService *api.FXService
}

func NewStatementNormalizer(fxService *api.FXService) *StatementNormalizer {
	return &StatementNormalizer{fxService: fxService}
}

// NormalizedPeriods contains the 3 historical annual periods and 1 LTM period.
type NormalizedPeriods struct {
	Historical []model.FinancialStatement // T-3, T-2, T-1
	LTM        model.FinancialStatement   // Trailing 12 months
}

// NormalizeStatements sorts, dedupes, selects the 3 most recent annual periods, and computes LTM.
func (n *StatementNormalizer) NormalizeStatements(
	ctx context.Context,
	statements []model.FinancialStatement,
	sourceCurrency, targetCurrency string,
) (*NormalizedPeriods, error) {
	var annuals []model.FinancialStatement
	var quarterlies []model.FinancialStatement

	annualMap := make(map[int]model.FinancialStatement)
	var ltmDirect *model.FinancialStatement

	for _, st := range statements {
		if st.PeriodType == model.PeriodAnnual && st.Revenue > 0 {
			if existing, ok := annualMap[st.FiscalYear]; !ok || (st.Revenue > existing.Revenue || st.NetIncome != 0) {
				annualMap[st.FiscalYear] = st
			}
		} else if st.PeriodType == model.PeriodQuarterly {
			quarterlies = append(quarterlies, st)
		} else if st.PeriodType == "LTM" || st.PeriodType == "TTM" {
			if ltmDirect == nil || st.PeriodEndDate > ltmDirect.PeriodEndDate {
				copied := st
				ltmDirect = &copied
			}
		}
	}

	for _, st := range annualMap {
		annuals = append(annuals, st)
	}

	sort.Slice(annuals, func(i, j int) bool {
		return annuals[i].FiscalYear < annuals[j].FiscalYear
	})

	// Select last 3 annual periods
	var hist []model.FinancialStatement
	if len(annuals) >= 3 {
		hist = annuals[len(annuals)-3:]
	} else {
		hist = annuals
	}

	// Compute LTM from direct trailing statement, trailing quarters, or fallback to latest annual
	var ltm model.FinancialStatement
	if ltmDirect != nil && ltmDirect.Revenue > 0 {
		ltm = *ltmDirect
		ltm.PeriodType = "LTM"
		ltm.FiscalPeriod = "LTM"
		if ltm.TotalEquity == 0 && len(hist) > 0 {
			lastAnn := hist[len(hist)-1]
			ltm.CashAndEquiv = lastAnn.CashAndEquiv
			ltm.TotalDebt = lastAnn.TotalDebt
			ltm.PreferredStock = lastAnn.PreferredStock
			ltm.TotalEquity = lastAnn.TotalEquity
		}
	} else {
		ltm = n.computeLTM(quarterlies, hist)
	}

	// Apply currency conversion if targetCurrency != sourceCurrency
	if targetCurrency != "" && sourceCurrency != "" && targetCurrency != sourceCurrency && n.fxService != nil {
		for i := range hist {
			n.convertStatement(ctx, &hist[i], sourceCurrency, targetCurrency)
		}
		n.convertStatement(ctx, &ltm, sourceCurrency, targetCurrency)
	}

	return &NormalizedPeriods{
		Historical: hist,
		LTM:        ltm,
	}, nil
}

func (n *StatementNormalizer) computeLTM(quarters []model.FinancialStatement, annuals []model.FinancialStatement) model.FinancialStatement {
	if len(quarters) >= 4 {
		// Sort quarterlies by end date or fiscal year/period
		sort.Slice(quarters, func(i, j int) bool {
			if quarters[i].FiscalYear != quarters[j].FiscalYear {
				return quarters[i].FiscalYear < quarters[j].FiscalYear
			}
			return quarters[i].FiscalPeriod < quarters[j].FiscalPeriod
		})

		last4 := quarters[len(quarters)-4:]
		latest := last4[3]

		var ltm model.FinancialStatement
		ltm.Ticker = latest.Ticker
		ltm.PeriodType = "LTM"
		ltm.FiscalYear = latest.FiscalYear
		ltm.FiscalPeriod = "LTM"
		ltm.PeriodEndDate = latest.PeriodEndDate
		ltm.UpdatedAt = time.Now()

		for _, q := range last4 {
			ltm.Revenue += q.Revenue
			ltm.CostOfRevenue += q.CostOfRevenue
			ltm.GrossProfit += q.GrossProfit
			ltm.OperatingIncome += q.OperatingIncome
			ltm.DepreciationAmortization += q.DepreciationAmortization
			ltm.NetIncome += q.NetIncome
			ltm.OperatingCashFlow += q.OperatingCashFlow
			ltm.CapEx += q.CapEx
			ltm.TaxExpense += q.TaxExpense
			ltm.PretaxIncome += q.PretaxIncome
		}

		if ltm.GrossProfit == 0 && ltm.Revenue > 0 && ltm.CostOfRevenue > 0 {
			ltm.GrossProfit = ltm.Revenue - ltm.CostOfRevenue
		}

		ltm.DilutedShares = latest.DilutedShares
		if ltm.DilutedShares > 0 {
			ltm.AdjEPS = ltm.NetIncome / ltm.DilutedShares
		} else {
			ltm.AdjEPS = latest.AdjEPS
		}

		// Balance sheet items come from latest quarter point-in-time
		ltm.CashAndEquiv = latest.CashAndEquiv
		ltm.TotalDebt = latest.TotalDebt
		ltm.PreferredStock = latest.PreferredStock
		ltm.TotalEquity = latest.TotalEquity

		// Fallback balance sheet and cash flows from latest annual if quarters missed or had incomplete data
		if len(annuals) > 0 {
			lastAnn := annuals[len(annuals)-1]
			if ltm.TotalEquity == 0 {
				ltm.CashAndEquiv = lastAnn.CashAndEquiv
				ltm.TotalDebt = lastAnn.TotalDebt
				ltm.PreferredStock = lastAnn.PreferredStock
				ltm.TotalEquity = lastAnn.TotalEquity
			}
			if ltm.OperatingCashFlow == 0 || (lastAnn.OperatingCashFlow != 0 && math.Abs(ltm.OperatingCashFlow) < 0.25*math.Abs(lastAnn.OperatingCashFlow)) {
				ltm.OperatingCashFlow = lastAnn.OperatingCashFlow
			}
			if ltm.CapEx == 0 || (lastAnn.CapEx != 0 && math.Abs(ltm.CapEx) < 0.25*math.Abs(lastAnn.CapEx)) {
				ltm.CapEx = lastAnn.CapEx
			}
			if ltm.DepreciationAmortization == 0 || (lastAnn.DepreciationAmortization != 0 && math.Abs(ltm.DepreciationAmortization) < 0.25*math.Abs(lastAnn.DepreciationAmortization)) {
				ltm.DepreciationAmortization = lastAnn.DepreciationAmortization
			}
		}

		return ltm
	}

	// Fallback to latest annual statement if insufficient quarters
	if len(annuals) > 0 {
		latest := annuals[len(annuals)-1]
		ltm := latest
		ltm.PeriodType = "LTM"
		ltm.FiscalPeriod = "LTM"
		return ltm
	}

	return model.FinancialStatement{
		PeriodType:   "LTM",
		FiscalPeriod: "LTM",
	}
}

func (n *StatementNormalizer) convertStatement(ctx context.Context, st *model.FinancialStatement, fromCurr, toCurr string) {
	avgRate, err := n.fxService.GetAverageRate(ctx, fromCurr, toCurr, st.FiscalYear)
	if err != nil || avgRate <= 0 {
		avgRate = 1.0
	}

	spotRate, err := n.fxService.GetSpotRate(ctx, fromCurr, toCurr)
	if err != nil || spotRate <= 0 {
		spotRate = 1.0
	}

	// Flow metrics convert by Average FX Rate
	st.Revenue *= avgRate
	st.CostOfRevenue *= avgRate
	st.GrossProfit *= avgRate
	st.OperatingIncome *= avgRate
	st.DepreciationAmortization *= avgRate
	st.NetIncome *= avgRate
	st.OperatingCashFlow *= avgRate
	st.CapEx *= avgRate
	st.TaxExpense *= avgRate
	st.PretaxIncome *= avgRate
	st.AdjEPS *= avgRate

	// Stock (balance sheet) metrics convert by Spot FX Rate
	st.CashAndEquiv *= spotRate
	st.TotalDebt *= spotRate
	st.PreferredStock *= spotRate
	st.TotalEquity *= spotRate
}

// ComputeCAGR computes compound annual growth rate clamped within [-5.0%, +25.0%].
func ComputeClampedCAGR(initialVal, finalVal float64, periods float64) float64 {
	if initialVal <= 0 || finalVal <= 0 || periods <= 0 {
		return 0.05 // default 5.0% conservative growth
	}
	cagr := math.Pow(finalVal/initialVal, 1.0/periods) - 1.0

	// Clamp to [-0.05, 0.25] (-5.0% to +25.0%)
	if cagr < -0.05 {
		return -0.05
	}
	if cagr > 0.25 {
		return 0.25
	}
	return cagr
}
