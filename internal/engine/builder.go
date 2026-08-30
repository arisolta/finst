package engine

import (
	"context"
	"math"

	"finst/internal/api"
	"finst/internal/model"
)

type DatasetBuilder struct {
	normalizer *StatementNormalizer
	forecaster *Forecaster
}

func NewDatasetBuilder(fxService *api.FXService) *DatasetBuilder {
	return &DatasetBuilder{
		normalizer: NewStatementNormalizer(fxService),
		forecaster: NewForecaster(),
	}
}

// BuildDataset constructs the complete 7-period FinancialDataset.
func (b *DatasetBuilder) BuildDataset(
	ctx context.Context,
	company model.CompanyInfo,
	price model.PriceValuation,
	statements []model.FinancialStatement,
	consensus []model.ConsensusEstimate,
	targetCurrency string,
) (*model.FinancialDataset, error) {
	sourceCurr := company.Currency
	if sourceCurr == "" {
		sourceCurr = "USD"
	}
	displayCurr := sourceCurr
	if targetCurrency != "" {
		displayCurr = targetCurrency
	}

	// Normalize historical statements and LTM
	norm, err := b.normalizer.NormalizeStatements(ctx, statements, sourceCurr, displayCurr)
	if err != nil {
		return nil, err
	}

	// Normalize price & valuation metrics to display currency
	origPriceCurr := price.Currency
	if origPriceCurr == "" {
		origPriceCurr = "USD"
	}

	if origPriceCurr != displayCurr && b.normalizer.fxService != nil {
		spot, err := b.normalizer.fxService.GetSpotRate(ctx, origPriceCurr, displayCurr)
		if err == nil && spot > 0 {
			price.SharePrice *= spot
			if price.MarketCap > 0 {
				price.MarketCap *= spot
			}
			if price.EnterpriseValue > 0 {
				price.EnterpriseValue *= spot
			}
			price.Currency = displayCurr
		}
	}

	// Normalize consensus estimates to display currency
	if displayCurr != "" && sourceCurr != "" && displayCurr != sourceCurr && b.normalizer.fxService != nil {
		spot, err := b.normalizer.fxService.GetSpotRate(ctx, sourceCurr, displayCurr)
		if err == nil && spot > 0 {
			for i := range consensus {
				consensus[i].EstRevenue *= spot
				consensus[i].EstEPS *= spot
			}
		}
	}

	hist := norm.Historical
	ltm := norm.LTM

	// Normalize historical share prices from trading currency (origPriceCurr) to display currency
	if origPriceCurr != displayCurr && b.normalizer.fxService != nil {
		for i := range hist {
			if hist[i].HistoricalPrice > 0 {
				avgRate, err := b.normalizer.fxService.GetAverageRate(ctx, origPriceCurr, displayCurr, hist[i].FiscalYear)
				if err != nil || avgRate <= 0 {
					avgRate, _ = b.normalizer.fxService.GetSpotRate(ctx, origPriceCurr, displayCurr)
				}
				if avgRate > 0 {
					hist[i].HistoricalPrice *= avgRate
				}
			}
		}
	}

	if price.SharesOutstanding == 0 && len(hist) > 0 {
		price.SharesOutstanding = hist[len(hist)-1].DilutedShares
		if price.MarketCap == 0 && price.SharePrice > 0 {
			price.MarketCap = price.SharePrice * price.SharesOutstanding
		}
	}
	if price.MarketCap == 0 && price.SharePrice > 0 && price.SharesOutstanding > 0 {
		price.MarketCap = price.SharePrice * price.SharesOutstanding
	}

	var periods []model.PeriodData

	// 1. Build Historical Periods (T-3, T-2, T-1)
	var prevRev float64
	var prevEPS float64

	for i, st := range hist {
		var pPrevRev, pPrevEPS float64
		if i > 0 {
			pPrevRev = hist[i-1].Revenue
			pPrevEPS = hist[i-1].AdjEPS
		}
		pData := BuildHistoricalPeriodData(st, false, pPrevRev, pPrevEPS, price)
		periods = append(periods, pData)
		prevRev = st.Revenue
		prevEPS = st.AdjEPS
	}

	// 2. Build LTM Period
	var ltmPrevRev, ltmPrevEPS float64
	if len(hist) > 0 {
		ltmPrevRev = hist[len(hist)-1].Revenue
		ltmPrevEPS = hist[len(hist)-1].AdjEPS
	}
	ltmData := BuildHistoricalPeriodData(ltm, true, ltmPrevRev, ltmPrevEPS, price)
	periods = append(periods, ltmData)

	// 3. Build Forward Projections (T+1, T+2, T+3)
	ratios := b.forecaster.Compute3YearRatios(hist)

	baseYear := 2025
	if len(hist) > 0 {
		baseYear = hist[len(hist)-1].FiscalYear
	}

	dilutedShares := price.SharesOutstanding
	if dilutedShares == 0 && len(hist) > 0 {
		dilutedShares = hist[len(hist)-1].DilutedShares
	}

	baseEquity := ltm.TotalEquity
	if baseEquity == 0 && len(hist) > 0 {
		baseEquity = hist[len(hist)-1].TotalEquity
	}

	// Index consensus estimates by fiscal year
	consMap := make(map[int]*model.ConsensusEstimate)
	for i := range consensus {
		consMap[consensus[i].FiscalYear] = &consensus[i]
	}

	fwdPrevRev := prevRev
	if fwdPrevRev == 0 && ltm.Revenue > 0 {
		fwdPrevRev = ltm.Revenue
	}
	fwdPrevEPS := prevEPS
	if fwdPrevEPS == 0 && ltm.AdjEPS != 0 {
		fwdPrevEPS = ltm.AdjEPS
	}

	// Calculate historical average dividend payout ratio
	var totalDivs, totalNI float64
	for _, st := range hist {
		if st.CashDividendsPaid > 0 && st.NetIncome > 0 {
			totalDivs += st.CashDividendsPaid
			totalNI += st.NetIncome
		}
	}
	var payoutRatio float64
	if totalNI > 0 && totalDivs > 0 {
		payoutRatio = totalDivs / totalNI
	} else if ltm.NetIncome > 0 && ltm.CashDividendsPaid > 0 {
		payoutRatio = ltm.CashDividendsPaid / ltm.NetIncome
	}
	if payoutRatio > 1.0 {
		payoutRatio = 1.0 // clamp to 100% for conservative forward projection
	}

	var forwardPeriods []model.PeriodData
	for step := 1; step <= 2; step++ {
		targetFY := baseYear + step
		var cons *model.ConsensusEstimate
		if c, ok := consMap[targetFY]; ok {
			cons = c
		}

		fwdData := b.forecaster.ProjectForwardYear(targetFY, fwdPrevRev, dilutedShares, ratios, cons, forwardPeriods)

		// Compute YoY growth & EPS growth for forward year
		if fwdPrevRev > 0 && fwdData.Revenue > 0 {
			growth := ((fwdData.Revenue - fwdPrevRev) / fwdPrevRev) * 100
			fwdData.YoYGrowthPct = &growth
		}
		if fwdPrevEPS != 0 && fwdData.DilutedAdjEPS != 0 {
			eg := ((fwdData.DilutedAdjEPS - fwdPrevEPS) / math.Abs(fwdPrevEPS)) * 100
			fwdData.EPSGrowthPct = &eg
		}

		// Calculate Forward Multiples
		PopulateForwardMultiples(&fwdData, price, baseEquity, payoutRatio)

		forwardPeriods = append(forwardPeriods, fwdData)
		periods = append(periods, fwdData)
		fwdPrevRev = fwdData.Revenue
		fwdPrevEPS = fwdData.DilutedAdjEPS
	}

	// Determine Scale (Millions by default for financial statement values)
	scaleUnit := "in Millions"
	scaleFactor := 1_000_000.0

	// Check if data is already in millions or raw dollars
	// If revenue > 1,000,000, we divide monetary aggregates by 1M
	needsScaling := false
	for _, p := range periods {
		if p.Revenue > 1_000_000 {
			needsScaling = true
			break
		}
	}

	if needsScaling {
		for i := range periods {
			p := &periods[i]
			p.Revenue /= scaleFactor
			p.GrossProfit /= scaleFactor
			p.EBITDA /= scaleFactor
			p.EBIT /= scaleFactor
			p.NetIncome /= scaleFactor
			p.OperatingCashFlow /= scaleFactor
			p.DepreciationAmortization /= scaleFactor
			p.CapEx /= scaleFactor
			p.FreeCashFlow /= scaleFactor

			if p.MarketCap != nil {
				scaled := *p.MarketCap / scaleFactor
				p.MarketCap = &scaled
			}
			if p.CashAndEquiv != nil {
				scaled := *p.CashAndEquiv / scaleFactor
				p.CashAndEquiv = &scaled
			}
			if p.PreferredAndOther != nil {
				scaled := *p.PreferredAndOther / scaleFactor
				p.PreferredAndOther = &scaled
			}
			if p.TotalDebt != nil {
				scaled := *p.TotalDebt / scaleFactor
				p.TotalDebt = &scaled
			}
			if p.EnterpriseValue != nil {
				scaled := *p.EnterpriseValue / scaleFactor
				p.EnterpriseValue = &scaled
			}
		}
	}

	return &model.FinancialDataset{
		Company:         company,
		Price:           price,
		DisplayCurrency: displayCurr,
		ScaleUnit:       scaleUnit,
		Periods:         periods,
	}, nil
}
