package engine

import (
	"fmt"
	"math"

	"finst/internal/model"
)

// CalculateEnterpriseValue computes EV = Market Cap + Total Debt + Preferred Stock - Cash & Equivalents.
func CalculateEnterpriseValue(marketCap, totalDebt, preferredStock, cashAndEquiv float64) float64 {
	ev := marketCap + totalDebt + preferredStock - cashAndEquiv
	if ev < 0 {
		return 0
	}
	return ev
}

// CalculateROE computes Return on Equity (%) = (Net Income / Total Stockholders' Equity) * 100.
func CalculateROE(netIncome, totalEquity float64) *float64 {
	if totalEquity <= 0 {
		return nil
	}
	val := (netIncome / totalEquity) * 100
	return &val
}

// CalculateROIC computes Return on Invested Capital (%).
// ROIC = (EBIT * (1 - Effective Tax Rate)) / (Total Debt + Total Equity - Cash & Equivalents) * 100.
// If effective tax rate is negative or anomalous, defaults to 21.0%.
func CalculateROIC(ebit, taxExpense, pretaxIncome, totalDebt, totalEquity, cashAndEquiv float64) *float64 {
	investedCapital := totalDebt + totalEquity - cashAndEquiv
	if investedCapital <= 0 {
		return nil
	}

	effectiveTaxRate := 0.21
	if pretaxIncome > 0 && taxExpense > 0 {
		rate := taxExpense / pretaxIncome
		if rate > 0 && rate < 0.50 {
			effectiveTaxRate = rate
		}
	}

	nopat := ebit * (1.0 - effectiveTaxRate)
	val := (nopat / investedCapital) * 100
	return &val
}

// CalculateFCF computes Free Cash Flow = CFO - |CapEx|.
func CalculateFCF(cfo, capex float64) float64 {
	return cfo - math.Abs(capex)
}

// BuildHistoricalPeriodData creates PeriodData for an annual or LTM statement.
func BuildHistoricalPeriodData(
	st model.FinancialStatement,
	isLTM bool,
	prevRev float64,
	prevEPS float64,
	currentPrice model.PriceValuation,
) model.PeriodData {
	label := fmt.Sprintf("%d Y", st.FiscalYear)
	periodType := model.PeriodTypeHistorical
	if isLTM {
		label = "LTM/Base"
		periodType = model.PeriodTypeLTM
	}

	ebitda := st.OperatingIncome + st.DepreciationAmortization
	fcf := CalculateFCF(st.OperatingCashFlow, st.CapEx)

	// Capital Structure
	var mktCap, ev, cash, pref, debt float64
	cash = st.CashAndEquiv
	pref = st.PreferredStock
	debt = st.TotalDebt

	shares := st.DilutedShares
	if shares == 0 {
		shares = currentPrice.SharesOutstanding
	}

	sharePrice := currentPrice.SharePrice
	if !isLTM && st.HistoricalPrice > 0 {
		sharePrice = st.HistoricalPrice
	}

	if shares > 0 && sharePrice > 0 {
		mktCap = sharePrice * shares
	} else if currentPrice.MarketCap > 0 {
		mktCap = currentPrice.MarketCap
	}

	ev = CalculateEnterpriseValue(mktCap, debt, pref, cash)

	// Percentages & Margins
	var yoyGrowth, epsGrowth, gmPct, ebitdaPct, ebitPct, netPct, fcfConvPct *float64

	if prevRev > 0 && st.Revenue > 0 {
		growth := ((st.Revenue - prevRev) / prevRev) * 100
		yoyGrowth = &growth
	}

	if prevEPS != 0 && st.AdjEPS != 0 {
		eg := ((st.AdjEPS - prevEPS) / math.Abs(prevEPS)) * 100
		epsGrowth = &eg
	}

	if st.Revenue > 0 {
		gm := (st.GrossProfit / st.Revenue) * 100
		gmPct = &gm

		em := (ebitda / st.Revenue) * 100
		ebitdaPct = &em

		ebm := (st.OperatingIncome / st.Revenue) * 100
		ebitPct = &ebm

		nm := (st.NetIncome / st.Revenue) * 100
		netPct = &nm
	}

	if st.NetIncome > 0 {
		conv := (fcf / st.NetIncome) * 100
		fcfConvPct = &conv
	}

	// ROE & ROIC
	roe := CalculateROE(st.NetIncome, st.TotalEquity)
	roic := CalculateROIC(st.OperatingIncome, st.TaxExpense, st.PretaxIncome, st.TotalDebt, st.TotalEquity, st.CashAndEquiv)

	// Multiples
	pe := CalculatePE(mktCap, st.NetIncome, sharePrice, st.AdjEPS)
	pb := CalculatePB(mktCap, st.TotalEquity)
	pfcf := CalculatePFCF(mktCap, fcf)
	evSales := CalculateEVSales(ev, st.Revenue)
	evEbitda := CalculateEVEBITDA(ev, ebitda)
	evEbit := CalculateEVEBIT(ev, st.OperatingIncome)

	// Cap Structure pointers
	mktCapPtr := &mktCap
	evPtr := &ev
	cashPtr := &cash
	prefPtr := &pref
	debtPtr := &debt

	// Ensure proper negative sign for CapEx
	capexVal := st.CapEx
	if capexVal > 0 {
		capexVal = -capexVal
	}

	return model.PeriodData{
		Label:                    label,
		FiscalYear:               st.FiscalYear,
		PeriodType:               periodType,
		IsForward:                false,
		MarketCap:                mktCapPtr,
		CashAndEquiv:             cashPtr,
		PreferredAndOther:        prefPtr,
		TotalDebt:                debtPtr,
		EnterpriseValue:          evPtr,
		Revenue:                  st.Revenue,
		YoYGrowthPct:             yoyGrowth,
		GrossProfit:              st.GrossProfit,
		GrossMarginPct:           gmPct,
		EBITDA:                   ebitda,
		EBITDAMarginPct:          ebitdaPct,
		EBIT:                     st.OperatingIncome,
		EBITMarginPct:            ebitPct,
		NetIncome:                st.NetIncome,
		NetMarginPct:             netPct,
		DilutedAdjEPS:            st.AdjEPS,
		EPSGrowthPct:             epsGrowth,
		OperatingCashFlow:        st.OperatingCashFlow,
		DepreciationAmortization: st.DepreciationAmortization,
		CapEx:                    capexVal,
		FreeCashFlow:             fcf,
		FCFConversionPct:         fcfConvPct,
		ROE:                      roe,
		ROIC:                     roic,
		PE:                       pe,
		PB:                       pb,
		PFCF:                     pfcf,
		EVSales:                  evSales,
		EVEBITDA:                 evEbitda,
		EVEBIT:                   evEbit,
	}
}
