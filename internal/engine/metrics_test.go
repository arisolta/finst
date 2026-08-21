package engine

import (
	"math"
	"testing"
)

func TestCalculateEnterpriseValue(t *testing.T) {
	tests := []struct {
		name           string
		marketCap      float64
		totalDebt      float64
		preferredStock float64
		cashAndEquiv   float64
		expectedEV     float64
	}{
		{
			name:           "Standard US Mega-Cap",
			marketCap:      120000.0,
			totalDebt:      12000.0,
			preferredStock: 240.0,
			cashAndEquiv:   1800.0,
			expectedEV:     130440.0,
		},
		{
			name:           "Net Cash Positive Company",
			marketCap:      50000.0,
			totalDebt:      5000.0,
			preferredStock: 0.0,
			cashAndEquiv:   20000.0,
			expectedEV:     35000.0,
		},
		{
			name:           "Zero Debt No Preferred",
			marketCap:      10000.0,
			totalDebt:      0.0,
			preferredStock: 0.0,
			cashAndEquiv:   1000.0,
			expectedEV:     9000.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := CalculateEnterpriseValue(tt.marketCap, tt.totalDebt, tt.preferredStock, tt.cashAndEquiv)
			if math.Abs(ev-tt.expectedEV) > 0.001 {
				t.Errorf("expected EV %.2f, got %.2f", tt.expectedEV, ev)
			}
		})
	}
}

func TestCalculateROE(t *testing.T) {
	// Positive equity
	roe := CalculateROE(2450.0, 19750.0)
	if roe == nil || math.Abs(*roe-12.405) > 0.01 {
		t.Errorf("expected ROE ~12.41%%, got %v", roe)
	}

	// Zero or Negative Equity edge case
	roeZero := CalculateROE(100.0, 0.0)
	if roeZero != nil {
		t.Errorf("expected nil for zero equity, got %v", *roeZero)
	}

	roeNeg := CalculateROE(100.0, -500.0)
	if roeNeg != nil {
		t.Errorf("expected nil for negative equity, got %v", *roeNeg)
	}
}

func TestCalculateROIC(t *testing.T) {
	// Standard calculation
	roic := CalculateROIC(3000.0, 600.0, 2800.0, 10000.0, 20000.0, 2000.0)
	if roic == nil || *roic <= 0 {
		t.Fatalf("expected positive ROIC, got %v", roic)
	}

	// Negative Invested Capital
	roicNegIC := CalculateROIC(3000.0, 0, 0, 1000.0, 1000.0, 5000.0)
	if roicNegIC != nil {
		t.Errorf("expected nil for negative invested capital, got %v", *roicNegIC)
	}
}

func TestCalculateFCF(t *testing.T) {
	cfo := 4500.0
	capex := -900.0
	fcf := CalculateFCF(cfo, capex)
	if fcf != 3600.0 {
		t.Errorf("expected FCF 3600.0, got %.2f", fcf)
	}

	// If capex is entered as positive number
	fcf2 := CalculateFCF(cfo, 900.0)
	if fcf2 != 3600.0 {
		t.Errorf("expected FCF 3600.0, got %.2f", fcf2)
	}
}
