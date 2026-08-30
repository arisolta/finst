package engine

import "finst/internal/model"

// CalculatePE computes P/E ratio. Returns nil if net income / EPS <= 0.
func CalculatePE(marketCap, netIncome, sharePrice, adjEPS float64) *float64 {
	if marketCap > 0 && netIncome > 0 {
		val := marketCap / netIncome
		return &val
	}
	if sharePrice > 0 && adjEPS > 0 {
		val := sharePrice / adjEPS
		return &val
	}
	return nil
}

// CalculatePB computes P/B ratio (Price-to-Book). Returns nil if total equity <= 0.
func CalculatePB(marketCap, totalEquity float64) *float64 {
	if marketCap <= 0 || totalEquity <= 0 {
		return nil
	}
	val := marketCap / totalEquity
	return &val
}

// CalculatePFCF computes P/FCF ratio. Returns nil if FCF <= 0.
func CalculatePFCF(marketCap, fcf float64) *float64 {
	if marketCap <= 0 || fcf <= 0 {
		return nil
	}
	val := marketCap / fcf
	return &val
}

// CalculateEVSales computes EV/Sales (Revenue). Returns nil if Revenue <= 0 or EV <= 0.
func CalculateEVSales(ev, revenue float64) *float64 {
	if ev <= 0 || revenue <= 0 {
		return nil
	}
	val := ev / revenue
	return &val
}

// CalculateEVEBITDA computes EV/EBITDA. Returns nil if EBITDA <= 0 or EV <= 0.
func CalculateEVEBITDA(ev, ebitda float64) *float64 {
	if ev <= 0 || ebitda <= 0 {
		return nil
	}
	val := ev / ebitda
	return &val
}

// CalculateEVEBIT computes EV/EBIT (Operating Income). Returns nil if EBIT <= 0 or EV <= 0.
func CalculateEVEBIT(ev, ebit float64) *float64 {
	if ev <= 0 || ebit <= 0 {
		return nil
	}
	val := ev / ebit
	return &val
}

// CalculateDividendYield computes Dividend Yield %. Returns nil if market cap <= 0 or dividends <= 0.
func CalculateDividendYield(marketCap, dividendsPaid float64) *float64 {
	if marketCap <= 0 || dividendsPaid <= 0 {
		return nil
	}
	val := (dividendsPaid / marketCap) * 100.0
	return &val
}

// PopulateForwardMultiples calculates valuation ratios for forward projected periods holding EV & Market Cap constant.
func PopulateForwardMultiples(
	p *model.PeriodData,
	currentPrice model.PriceValuation,
	baseEquity float64,
	payoutRatio float64,
) {
	mktCap := currentPrice.MarketCap
	if mktCap == 0 && currentPrice.SharePrice > 0 && p.DilutedAdjEPS > 0 {
		// If market cap not populated, share price / EPS is used
	}

	ev := currentPrice.EnterpriseValue
	if ev == 0 && mktCap > 0 {
		ev = mktCap
	}

	p.PE = CalculatePE(mktCap, p.NetIncome, currentPrice.SharePrice, p.DilutedAdjEPS)
	p.PB = CalculatePB(mktCap, baseEquity)
	p.PFCF = CalculatePFCF(mktCap, p.FreeCashFlow)
	p.EVSales = CalculateEVSales(ev, p.Revenue)
	p.EVEBITDA = CalculateEVEBITDA(ev, p.EBITDA)

	ebit := p.EBITDA - p.DepreciationAmortization
	p.EVEBIT = CalculateEVEBIT(ev, ebit)

	if payoutRatio > 0 && p.NetIncome > 0 && mktCap > 0 {
		fwdDividends := p.NetIncome * payoutRatio
		p.DividendYieldPct = CalculateDividendYield(mktCap, fwdDividends)
	}

	// In forward years, capital structure line items are not projected, so leave as nil
	p.MarketCap = nil
	p.CashAndEquiv = nil
	p.PreferredAndOther = nil
	p.TotalDebt = nil
	p.EnterpriseValue = nil
}
