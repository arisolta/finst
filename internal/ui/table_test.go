package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/arisolta/finst/internal/model"
)

func TestRenderScreen(t *testing.T) {
	SetColorsEnabled(false) // disable ANSI for deterministic assertion

	mktCap := 121450.0
	cash := 1820.0
	pref := 240.0
	debt := 12255.0
	ev := 131885.0
	growth := 13.5
	gm := 71.0
	em := 30.5
	netPct := 19.2
	eg := 16.2
	fcfConv := 89.7
	roe := 16.2
	roic := 13.1
	pe := 30.1
	pb := 3.9
	pfcf := 33.5
	evSales := 6.3
	evEbitda := 20.6
	evEbit := 26.1

	dataset := &model.FinancialDataset{
		Company: model.CompanyInfo{
			Ticker:            "BSX",
			Name:              "Boston Scientific Corp",
			Exchange:          "US Equity",
			Sector:            "Healthcare",
			Industry:          "Medical Devices",
			Currency:          "USD",
			ReportingStandard: "GAAP / SEC EDGAR",
			UpdatedAt:         time.Now(),
		},
		Price: model.PriceValuation{
			SharePrice: 82.40,
			MarketCap:  121450.0,
		},
		DisplayCurrency: "USD",
		ScaleUnit:       "in Millions",
		Periods: []model.PeriodData{
			{
				Label:      "2023 Y",
				FiscalYear: 2023,
				Revenue:    14240.0,
			},
			{
				Label:      "2024 Y",
				FiscalYear: 2024,
				Revenue:    16740.0,
			},
			{
				Label:      "2025 Y",
				FiscalYear: 2025,
				Revenue:    20074.0,
			},
			{
				Label:                    "LTM/Base",
				FiscalYear:               2025,
				MarketCap:                &mktCap,
				CashAndEquiv:             &cash,
				PreferredAndOther:        &pref,
				TotalDebt:                &debt,
				EnterpriseValue:          &ev,
				Revenue:                  20996.0,
				YoYGrowthPct:             &growth,
				GrossProfit:              14904.0,
				GrossMarginPct:           &gm,
				EBITDA:                   6408.0,
				EBITDAMarginPct:          &em,
				EBIT:                     5048.0,
				EBITMarginPct:            &em,
				NetIncome:                4039.0,
				NetMarginPct:             &netPct,
				DilutedAdjEPS:            2.72,
				EPSGrowthPct:             &eg,
				OperatingCashFlow:        4529.0,
				DepreciationAmortization: 1360.0,
				CapEx:                    -904.0,
				FreeCashFlow:             3625.0,
				FCFConversionPct:         &fcfConv,
				ROE:                      &roe,
				ROIC:                     &roic,
				PE:                       &pe,
				PB:                       &pb,
				PFCF:                     &pfcf,
				EVSales:                  &evSales,
				EVEBITDA:                 &evEbitda,
				EVEBIT:                   &evEbit,
			},
			{
				Label:      "2026E (Cons)",
				FiscalYear: 2026,
				Revenue:    21341.8,
			},
			{
				Label:      "2027E (Cons)",
				FiscalYear: 2027,
				Revenue:    22275.2,
			},
		},
	}

	out := RenderScreen(dataset, model.ViewStandard)
	if !strings.Contains(out, "BSX US Equity") {
		t.Errorf("output missing ticker header")
	}
	if !strings.Contains(out, "[CAPITAL STRUCTURE]") {
		t.Errorf("output missing [CAPITAL STRUCTURE]")
	}
	if !strings.Contains(out, "[OPERATING PERFORMANCE]") {
		t.Errorf("output missing [OPERATING PERFORMANCE]")
	}
	if !strings.Contains(out, "EBIT") {
		t.Errorf("output missing EBIT")
	}
	if !strings.Contains(out, "[VALUATION MULTIPLES]") {
		t.Errorf("output missing [VALUATION MULTIPLES]")
	}
	if !strings.Contains(out, "20,996.0") {
		t.Errorf("output missing formatted revenue 20,996.0")
	}

	// Test JSON export
	jsonStr, err := RenderJSON(dataset)
	if err != nil || !strings.Contains(jsonStr, `"ticker": "BSX"`) {
		t.Errorf("invalid json output: %v", err)
	}

	// Test CSV export
	csvStr, err := RenderCSV(dataset)
	if err != nil || !strings.Contains(csvStr, "Line Item,2023 Y,2024 Y,2025 Y,LTM/Base") {
		t.Errorf("invalid csv output: %v", err)
	}
}
