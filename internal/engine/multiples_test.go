package engine

import (
	"testing"

	"finst/internal/ui"
)

func TestMultiplesNegativeDenominators(t *testing.T) {
	// P/E with negative Net Income / EPS
	pe := CalculatePE(100000.0, -500.0, 50.0, -0.25)
	if pe != nil {
		t.Errorf("expected nil for negative earnings, got %v", *pe)
	}
	if str := ui.FormatMultiple(pe); str != "N/A" {
		t.Errorf("expected format 'N/A', got '%s'", str)
	}

	// P/B with negative Equity
	pb := CalculatePB(100000.0, -1000.0)
	if pb != nil {
		t.Errorf("expected nil for negative equity, got %v", *pb)
	}
	if str := ui.FormatMultiple(pb); str != "N/A" {
		t.Errorf("expected format 'N/A', got '%s'", str)
	}

	// P/FCF with negative FCF
	pfcf := CalculatePFCF(100000.0, -200.0)
	if pfcf != nil {
		t.Errorf("expected nil for negative FCF, got %v", *pfcf)
	}
	if str := ui.FormatMultiple(pfcf); str != "N/A" {
		t.Errorf("expected format 'N/A', got '%s'", str)
	}

	// EV/EBITDA with negative EBITDA
	evEbitda := CalculateEVEBITDA(120000.0, -50.0)
	if evEbitda != nil {
		t.Errorf("expected nil for negative EBITDA, got %v", *evEbitda)
	}
	if str := ui.FormatMultiple(evEbitda); str != "N/A" {
		t.Errorf("expected format 'N/A', got '%s'", str)
	}
}

func TestMultiplesPositiveValues(t *testing.T) {
	// Standard P/E: market cap 100,000 / net income 4,000 = 25.0
	pe := CalculatePE(100000.0, 4000.0, 50.0, 2.0)
	if pe == nil || *pe != 25.0 {
		t.Fatalf("expected P/E 25.0, got %v", pe)
	}
	if str := ui.FormatMultiple(pe); str != "25.0x" {
		t.Errorf("expected format '25.0x', got '%s'", str)
	}

	// Standard EV/Sales
	evSales := CalculateEVSales(100000.0, 20000.0)
	if evSales == nil || *evSales != 5.0 {
		t.Fatalf("expected EV/Sales 5.0, got %v", evSales)
	}
	if str := ui.FormatMultiple(evSales); str != "5.0x" {
		t.Errorf("expected format '5.0x', got '%s'", str)
	}

	// Standard EV/EBITDA
	evEbitda := CalculateEVEBITDA(100000.0, 5000.0)
	if evEbitda == nil || *evEbitda != 20.0 {
		t.Fatalf("expected EV/EBITDA 20.0, got %v", evEbitda)
	}
	if str := ui.FormatMultiple(evEbitda); str != "20.0x" {
		t.Errorf("expected format '20.0x', got '%s'", str)
	}

	// Standard EV/EBIT
	evEbit := CalculateEVEBIT(100000.0, 4000.0)
	if evEbit == nil || *evEbit != 25.0 {
		t.Fatalf("expected EV/EBIT 25.0, got %v", evEbit)
	}
	if str := ui.FormatMultiple(evEbit); str != "25.0x" {
		t.Errorf("expected format '25.0x', got '%s'", str)
	}
}
