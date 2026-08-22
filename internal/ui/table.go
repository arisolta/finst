package ui

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"finst/internal/model"
)

const (
	LineWidth  = 120
	ColItemW   = 26
	ColPeriodW = 13
)

// RenderScreen renders the full terminal Bloomberg-style table.
func RenderScreen(ds *model.FinancialDataset, viewMode string) string {
	var sb strings.Builder

	doubleLine := strings.Repeat("=", LineWidth)
	singleLine := strings.Repeat("-", LineWidth)

	// Top banner
	sb.WriteString(doubleLine + "\n")

	sectorInd := ds.Company.Sector
	if ds.Company.Industry != "" && ds.Company.Sector != "" {
		sectorInd = fmt.Sprintf("%s / %s", ds.Company.Sector, ds.Company.Industry)
	} else if sectorInd == "" {
		sectorInd = ds.Company.Exchange
	}

	unitStr := fmt.Sprintf("%s (%s)", ds.DisplayCurrency, ds.ScaleUnit)
	exchangeStr := ds.Company.Exchange
	if exchangeStr == "" {
		exchangeStr = "Equity"
	}
	topLine := fmt.Sprintf(" %s %s  |  %s  |  %s  |  %s",
		ds.Company.Ticker, exchangeStr, ds.Company.Name, sectorInd, unitStr)
	sb.WriteString(Colorize(ColorHeader, topLine) + "\n")

	stdStr := "Standard: "
	if ds.Company.ReportingStandard != "" {
		stdStr += ds.Company.ReportingStandard
	} else if strings.HasSuffix(ds.Company.Ticker, ".PA") || strings.HasSuffix(ds.Company.Ticker, ".DE") || strings.HasSuffix(ds.Company.Ticker, ".L") {
		stdStr += "IFRS"
	} else {
		stdStr += "GAAP / SEC EDGAR"
	}

	priceStr := FormatPrice(ds.Price.SharePrice, ds.DisplayCurrency)
	mktCapStr := FormatNullableNumber(&ds.Periods[3].Revenue, 1) // placeholder or actual
	if ds.Periods[3].MarketCap != nil {
		mktCapStr = FormatNullableNumber(ds.Periods[3].MarketCap, 1)
	}
	evStr := FormatNullableNumber(ds.Periods[3].EnterpriseValue, 1)

	subBanner := fmt.Sprintf(" Share Price: %s  |  Market Cap: %s  |  Enterprise Value: %s  |  %s",
		priceStr, mktCapStr, evStr, stdStr)
	sb.WriteString(Colorize(ColorSlate, subBanner) + "\n")
	sb.WriteString(doubleLine + "\n")

	// Table Header
	headerRow := fmt.Sprintf(" %-*s", ColItemW, "Line Item")
	for _, p := range ds.Periods {
		headerRow += fmt.Sprintf(" %*s", ColPeriodW, p.Label)
	}
	sb.WriteString(Colorize(ColorHeader, headerRow) + "\n")
	sb.WriteString(singleLine + "\n")

	if viewMode == model.ViewCompact {
		renderCompactRows(&sb, ds)
	} else {
		renderStandardRows(&sb, ds)
	}

	sb.WriteString(doubleLine + "\n")
	return sb.String()
}

func renderStandardRows(sb *strings.Builder, ds *model.FinancialDataset) {
	singleLine := strings.Repeat("-", LineWidth)

	// Section 1: CAPITAL STRUCTURE
	sb.WriteString(Colorize(ColorSection, " [CAPITAL STRUCTURE]") + "\n")
	printRowNullable(sb, "Market Capitalization", ds.Periods, 1, func(p model.PeriodData) *float64 { return p.MarketCap })
	printRowNullable(sb, "- Cash & Equivalents", ds.Periods, 1, func(p model.PeriodData) *float64 { return p.CashAndEquiv })
	printRowNullable(sb, "+ Preferred & Other", ds.Periods, 1, func(p model.PeriodData) *float64 { return p.PreferredAndOther })
	printRowNullable(sb, "+ Total Debt", ds.Periods, 1, func(p model.PeriodData) *float64 { return p.TotalDebt })
	printRowNullable(sb, "Enterprise Value", ds.Periods, 1, func(p model.PeriodData) *float64 { return p.EnterpriseValue })
	sb.WriteString(singleLine + "\n")

	// Section 2: OPERATING PERFORMANCE
	sb.WriteString(Colorize(ColorSection, " [OPERATING PERFORMANCE]") + "\n")
	printRow(sb, "Revenue", ds.Periods, 1, func(p model.PeriodData) float64 { return p.Revenue })
	printRowPct(sb, "  YoY Growth %", ds.Periods, func(p model.PeriodData) *float64 { return p.YoYGrowthPct })
	printRow(sb, "Gross Profit", ds.Periods, 1, func(p model.PeriodData) float64 { return p.GrossProfit })
	printRowPct(sb, "  Gross Margin %", ds.Periods, func(p model.PeriodData) *float64 { return p.GrossMarginPct })
	printRow(sb, "EBITDA", ds.Periods, 1, func(p model.PeriodData) float64 { return p.EBITDA })
	printRowPct(sb, "  EBITDA Margin %", ds.Periods, func(p model.PeriodData) *float64 { return p.EBITDAMarginPct })
	printRow(sb, "EBIT", ds.Periods, 1, func(p model.PeriodData) float64 { return p.EBIT })
	printRowPct(sb, "  EBIT Margin %", ds.Periods, func(p model.PeriodData) *float64 { return p.EBITMarginPct })
	printRow(sb, "Net Income", ds.Periods, 1, func(p model.PeriodData) float64 { return p.NetIncome })
	printRowPct(sb, "  Net Margin %", ds.Periods, func(p model.PeriodData) *float64 { return p.NetMarginPct })
	printRowEPS(sb, "Diluted Adj. EPS", ds.Periods, func(p model.PeriodData) float64 { return p.DilutedAdjEPS })
	printRowPct(sb, "  EPS Growth %", ds.Periods, func(p model.PeriodData) *float64 { return p.EPSGrowthPct })
	sb.WriteString(singleLine + "\n")

	// Section 3: CASH FLOW PROFILE
	sb.WriteString(Colorize(ColorSection, " [CASH FLOW PROFILE]") + "\n")
	printRow(sb, "Cash from Operations", ds.Periods, 1, func(p model.PeriodData) float64 { return p.OperatingCashFlow })
	printRow(sb, "  (-) D&A", ds.Periods, 1, func(p model.PeriodData) float64 { return p.DepreciationAmortization })
	printRow(sb, "Capital Expenditures", ds.Periods, 1, func(p model.PeriodData) float64 { return p.CapEx })
	printRow(sb, "Free Cash Flow", ds.Periods, 1, func(p model.PeriodData) float64 { return p.FreeCashFlow })
	printRowPct(sb, "  FCF Conversion %", ds.Periods, func(p model.PeriodData) *float64 { return p.FCFConversionPct })
	sb.WriteString(singleLine + "\n")

	// Section 4: RETURNS & PROFITABILITY
	sb.WriteString(Colorize(ColorSection, " [RETURNS & PROFITABILITY]") + "\n")
	printRowPct(sb, "Return on Equity (ROE)", ds.Periods, func(p model.PeriodData) *float64 { return p.ROE })
	printRowPct(sb, "Return on Inv. Cap (ROIC)", ds.Periods, func(p model.PeriodData) *float64 { return p.ROIC })
	sb.WriteString(singleLine + "\n")

	// Section 5: VALUATION MULTIPLES
	sb.WriteString(Colorize(ColorSection, " [VALUATION MULTIPLES]") + "\n")
	printRowMultiple(sb, "P/E", ds.Periods, func(p model.PeriodData) *float64 { return p.PE })
	printRowMultiple(sb, "P/B", ds.Periods, func(p model.PeriodData) *float64 { return p.PB })
	printRowMultiple(sb, "P/FCF", ds.Periods, func(p model.PeriodData) *float64 { return p.PFCF })
	printRowMultiple(sb, "EV/EBITDA", ds.Periods, func(p model.PeriodData) *float64 { return p.EVEBITDA })
	printRowMultiple(sb, "EV/EBIT", ds.Periods, func(p model.PeriodData) *float64 { return p.EVEBIT })
	printRowMultiple(sb, "EV/Book", ds.Periods, func(p model.PeriodData) *float64 { return p.EVBook })
}

func renderCompactRows(sb *strings.Builder, ds *model.FinancialDataset) {
	singleLine := strings.Repeat("-", LineWidth)

	sb.WriteString(Colorize(ColorSection, " [KEY FINANCIALS]") + "\n")
	printRow(sb, "Revenue", ds.Periods, 1, func(p model.PeriodData) float64 { return p.Revenue })
	printRow(sb, "Gross Profit", ds.Periods, 1, func(p model.PeriodData) float64 { return p.GrossProfit })
	printRow(sb, "EBITDA", ds.Periods, 1, func(p model.PeriodData) float64 { return p.EBITDA })
	printRow(sb, "EBIT", ds.Periods, 1, func(p model.PeriodData) float64 { return p.EBIT })
	printRow(sb, "Net Income", ds.Periods, 1, func(p model.PeriodData) float64 { return p.NetIncome })
	printRowEPS(sb, "Diluted Adj. EPS", ds.Periods, func(p model.PeriodData) float64 { return p.DilutedAdjEPS })
	printRow(sb, "Free Cash Flow", ds.Periods, 1, func(p model.PeriodData) float64 { return p.FreeCashFlow })
	sb.WriteString(singleLine + "\n")

	sb.WriteString(Colorize(ColorSection, " [VALUATION MULTIPLES]") + "\n")
	printRowMultiple(sb, "P/E", ds.Periods, func(p model.PeriodData) *float64 { return p.PE })
	printRowMultiple(sb, "EV/EBITDA", ds.Periods, func(p model.PeriodData) *float64 { return p.EVEBITDA })
	printRowMultiple(sb, "P/FCF", ds.Periods, func(p model.PeriodData) *float64 { return p.PFCF })
}

func printRow(sb *strings.Builder, name string, periods []model.PeriodData, decimals int, getter func(model.PeriodData) float64) {
	row := fmt.Sprintf(" %-*s", ColItemW, name)
	for _, p := range periods {
		val := getter(p)
		row += fmt.Sprintf(" %*s", ColPeriodW, FormatNumber(val, decimals))
	}
	sb.WriteString(row + "\n")
}

func printRowNullable(sb *strings.Builder, name string, periods []model.PeriodData, decimals int, getter func(model.PeriodData) *float64) {
	row := fmt.Sprintf(" %-*s", ColItemW, name)
	for _, p := range periods {
		ptr := getter(p)
		row += fmt.Sprintf(" %*s", ColPeriodW, FormatNullableNumber(ptr, decimals))
	}
	sb.WriteString(row + "\n")
}

func printRowPct(sb *strings.Builder, name string, periods []model.PeriodData, getter func(model.PeriodData) *float64) {
	row := fmt.Sprintf(" %-*s", ColItemW, name)
	for _, p := range periods {
		ptr := getter(p)
		formatted := FormatPercentage(ptr)
		if ptr != nil && *ptr > 0 {
			// optional accent
		}
		row += fmt.Sprintf(" %*s", ColPeriodW, formatted)
	}
	sb.WriteString(row + "\n")
}

func printRowEPS(sb *strings.Builder, name string, periods []model.PeriodData, getter func(model.PeriodData) float64) {
	row := fmt.Sprintf(" %-*s", ColItemW, name)
	for _, p := range periods {
		val := getter(p)
		row += fmt.Sprintf(" %*s", ColPeriodW, FormatEPS(val))
	}
	sb.WriteString(row + "\n")
}

func printRowMultiple(sb *strings.Builder, name string, periods []model.PeriodData, getter func(model.PeriodData) *float64) {
	row := fmt.Sprintf(" %-*s", ColItemW, name)
	for _, p := range periods {
		ptr := getter(p)
		row += fmt.Sprintf(" %*s", ColPeriodW, FormatMultiple(ptr))
	}
	sb.WriteString(row + "\n")
}

// RenderJSON formats dataset as indented JSON.
func RenderJSON(ds *model.FinancialDataset) (string, error) {
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RenderCSV formats dataset as CSV table.
func RenderCSV(ds *model.FinancialDataset) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header row
	header := []string{"Line Item"}
	for _, p := range ds.Periods {
		header = append(header, p.Label)
	}
	if err := w.Write(header); err != nil {
		return "", err
	}

	addRow := func(name string, getter func(model.PeriodData) string) {
		row := []string{name}
		for _, p := range ds.Periods {
			row = append(row, getter(p))
		}
		_ = w.Write(row)
	}

	addRow("Market Capitalization", func(p model.PeriodData) string { return FormatNullableNumber(p.MarketCap, 1) })
	addRow("Cash & Equivalents", func(p model.PeriodData) string { return FormatNullableNumber(p.CashAndEquiv, 1) })
	addRow("Preferred & Other", func(p model.PeriodData) string { return FormatNullableNumber(p.PreferredAndOther, 1) })
	addRow("Total Debt", func(p model.PeriodData) string { return FormatNullableNumber(p.TotalDebt, 1) })
	addRow("Enterprise Value", func(p model.PeriodData) string { return FormatNullableNumber(p.EnterpriseValue, 1) })

	addRow("Revenue", func(p model.PeriodData) string { return FormatNumber(p.Revenue, 1) })
	addRow("YoY Growth %", func(p model.PeriodData) string { return FormatPercentage(p.YoYGrowthPct) })
	addRow("Gross Profit", func(p model.PeriodData) string { return FormatNumber(p.GrossProfit, 1) })
	addRow("Gross Margin %", func(p model.PeriodData) string { return FormatPercentage(p.GrossMarginPct) })
	addRow("EBITDA", func(p model.PeriodData) string { return FormatNumber(p.EBITDA, 1) })
	addRow("EBITDA Margin %", func(p model.PeriodData) string { return FormatPercentage(p.EBITDAMarginPct) })
	addRow("EBIT", func(p model.PeriodData) string { return FormatNumber(p.EBIT, 1) })
	addRow("EBIT Margin %", func(p model.PeriodData) string { return FormatPercentage(p.EBITMarginPct) })
	addRow("Net Income", func(p model.PeriodData) string { return FormatNumber(p.NetIncome, 1) })
	addRow("Net Margin %", func(p model.PeriodData) string { return FormatPercentage(p.NetMarginPct) })
	addRow("Diluted Adj. EPS", func(p model.PeriodData) string { return FormatEPS(p.DilutedAdjEPS) })
	addRow("EPS Growth %", func(p model.PeriodData) string { return FormatPercentage(p.EPSGrowthPct) })

	addRow("Cash from Operations", func(p model.PeriodData) string { return FormatNumber(p.OperatingCashFlow, 1) })
	addRow("Depreciation & Amortization", func(p model.PeriodData) string { return FormatNumber(p.DepreciationAmortization, 1) })
	addRow("Capital Expenditures", func(p model.PeriodData) string { return FormatNumber(p.CapEx, 1) })
	addRow("Free Cash Flow", func(p model.PeriodData) string { return FormatNumber(p.FreeCashFlow, 1) })
	addRow("FCF Conversion %", func(p model.PeriodData) string { return FormatPercentage(p.FCFConversionPct) })

	addRow("Return on Equity (ROE)", func(p model.PeriodData) string { return FormatPercentage(p.ROE) })
	addRow("Return on Inv. Cap (ROIC)", func(p model.PeriodData) string { return FormatPercentage(p.ROIC) })

	addRow("P/E", func(p model.PeriodData) string { return FormatMultiple(p.PE) })
	addRow("P/B", func(p model.PeriodData) string { return FormatMultiple(p.PB) })
	addRow("P/FCF", func(p model.PeriodData) string { return FormatMultiple(p.PFCF) })
	addRow("EV/EBITDA", func(p model.PeriodData) string { return FormatMultiple(p.EVEBITDA) })
	addRow("EV/EBIT", func(p model.PeriodData) string { return FormatMultiple(p.EVEBIT) })
	addRow("EV/Book", func(p model.PeriodData) string { return FormatMultiple(p.EVBook) })

	w.Flush()
	return buf.String(), nil
}
