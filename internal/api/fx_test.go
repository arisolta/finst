package api

import (
	"context"
	"testing"
)

func TestFXServiceSameCurrency(t *testing.T) {
	client := NewClient()
	fx := NewFXService(client)
	ctx := context.Background()

	rate, err := fx.GetSpotRate(ctx, "USD", "USD")
	if err != nil {
		t.Fatalf("expected no error for same currency, got: %v", err)
	}
	if rate != 1.0 {
		t.Errorf("expected rate 1.0, got %f", rate)
	}
}

func TestFXServiceYahooFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in short mode")
	}

	client := NewClient()
	fx := NewFXService(client)
	ctx := context.Background()

	// KZT is not supported by Frankfurter, will test Yahoo FX fallback
	rate, err := fx.GetSpotRate(ctx, "USD", "KZT")
	if err != nil {
		t.Fatalf("failed to fetch USD -> KZT rate via Yahoo fallback: %v", err)
	}
	if rate < 300 || rate > 1000 {
		t.Errorf("expected USD -> KZT rate roughly between 300 and 1000, got %f", rate)
	}

	// Test inverse KZT -> USD
	invRate, err := fx.GetSpotRate(ctx, "KZT", "USD")
	if err != nil {
		t.Fatalf("failed to fetch KZT -> USD rate: %v", err)
	}
	if invRate <= 0 || invRate > 0.01 {
		t.Errorf("expected KZT -> USD rate roughly between 0 and 0.01, got %f", invRate)
	}
}
