package engine

import (
	"math"
	"testing"

	"github.com/arisolta/finst/internal/model"
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
	p2026 := forecaster.ProjectForwardYear(2026, 12100, 1000, ratios, nil, nil)
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

func TestOption2BlendedSmoothing(t *testing.T) {
	forecaster := NewForecaster()

	hist := []model.FinancialStatement{
		{FiscalYear: 2023, Revenue: 10000, GrossProfit: 7000, OperatingIncome: 2000, DepreciationAmortization: 1000, NetIncome: 1500, CapEx: -500},
		{FiscalYear: 2024, Revenue: 11000, GrossProfit: 7700, OperatingIncome: 2200, DepreciationAmortization: 1100, NetIncome: 1650, CapEx: -550},
		{FiscalYear: 2025, Revenue: 12100, GrossProfit: 8470, OperatingIncome: 2420, DepreciationAmortization: 1210, NetIncome: 1815, CapEx: -605},
	}
	ratios := forecaster.Compute3YearRatios(hist)

	// Step 1: 2026E Consensus (Rev 13000, EPS 2.50)
	cons2026 := &model.ConsensusEstimate{FiscalYear: 2026, EstRevenue: 13000, EstEPS: 2.50}
	p2026 := forecaster.ProjectForwardYear(2026, 12100, 1000, ratios, cons2026, nil)

	// Step 2: 2027E Consensus (Rev 14000, EPS 3.00)
	cons2027 := &model.ConsensusEstimate{FiscalYear: 2027, EstRevenue: 14000, EstEPS: 3.00}
	p2027 := forecaster.ProjectForwardYear(2027, 13000, 1000, ratios, cons2027, []model.PeriodData{p2026})

	// Step 3: 2028E Blended Projection
	p2028 := forecaster.ProjectForwardYear(2028, 14000, 1000, ratios, nil, []model.PeriodData{p2026, p2027})

	if p2028.PeriodType != model.PeriodTypeProjection {
		t.Errorf("expected PROJECTION period type, got %s", p2028.PeriodType)
	}

	// 2028 Revenue should grow smoothly from 14,000 (around 7.7% - 10%)
	if p2028.Revenue <= p2027.Revenue {
		t.Errorf("expected 2028 revenue > 2027 revenue, got %f vs %f", p2028.Revenue, p2027.Revenue)
	}

	// 2028 Net Income should NOT experience a cliff drop
	if p2028.NetIncome < p2027.NetIncome*0.95 {
		t.Errorf("expected smooth net income transition, got %f vs 2027 net income %f", p2028.NetIncome, p2027.NetIncome)
	}
}
