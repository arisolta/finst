package engine

import (
	"math"
	"testing"

	"finst/internal/model"
)

func TestCAGRBoundsClamping(t *testing.T) {
	// Triple-digit spike (e.g. 100 to 1000 over 2 years = +216%) -> should clamp to +25.0%
	cagrHigh := ComputeClampedCAGR(100.0, 1000.0, 2.0)
	if math.Abs(cagrHigh-0.25) > 0.0001 {
		t.Errorf("expected CAGR clamped to 0.25 (25%%), got %f", cagrHigh)
	}

	// Negative collapse (e.g. 1000 to 100 over 2 years = -68%) -> should clamp to -5.0%
	cagrLow := ComputeClampedCAGR(1000.0, 100.0, 2.0)
	if math.Abs(cagrLow - (-0.05)) > 0.0001 {
		t.Errorf("expected CAGR clamped to -0.05 (-5%%), got %f", cagrLow)
	}

	// Normal growth (e.g. 100 to 121 over 2 years = +10.0%) -> should remain +10.0%
	cagrNormal := ComputeClampedCAGR(100.0, 121.0, 2.0)
	if math.Abs(cagrNormal-0.10) > 0.001 {
		t.Errorf("expected CAGR ~0.10 (10%%), got %f", cagrNormal)
	}
}

func TestForecasterProjection(t *testing.T) {
	forecaster := NewForecaster()

	hist := []model.FinancialStatement{
		{FiscalYear: 2023, Revenue: 10000, GrossProfit: 7000, OperatingIncome: 2000, DepreciationAmortization: 1000, NetIncome: 1500, CapEx: -500},
		{FiscalYear: 2024, Revenue: 11000, GrossProfit: 7700, OperatingIncome: 2200, DepreciationAmortization: 1100, NetIncome: 1650, CapEx: -550},
		{FiscalYear: 2025, Revenue: 12100, GrossProfit: 8470, OperatingIncome: 2420, DepreciationAmortization: 1210, NetIncome: 1815, CapEx: -605},
	}

	ratios := forecaster.Compute3YearRatios(hist)
	if math.Abs(ratios.CAGR-0.10) > 0.01 {
		t.Errorf("expected ~10%% CAGR, got %f", ratios.CAGR)
	}

	// Project 2026 without consensus (heuristic fallback)
	p2026 := forecaster.ProjectForwardYear(2026, 12100, 1000, ratios, nil)
	if p2026.PeriodType != model.PeriodTypeProjection {
		t.Errorf("expected PROJECTION period type, got %s", p2026.PeriodType)
	}
	if math.Abs(p2026.Revenue-13310) > 10.0 {
		t.Errorf("expected ~13,310 revenue, got %f", p2026.Revenue)
	}
	if p2026.FreeCashFlow <= 0 {
		t.Errorf("expected positive FCF, got %f", p2026.FreeCashFlow)
	}
}
