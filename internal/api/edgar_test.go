package api

import (
	"context"
	"testing"
)

func TestEdgarAMZN(t *testing.T) {
	client := NewClient()
	service := NewEdgarService(client)
	ctx := context.Background()

	cik, title, err := service.ResolveTicker(ctx, "AMZN")
	if err != nil || cik == "" {
		t.Fatalf("failed to resolve AMZN: %v", err)
	}
	t.Logf("AMZN resolved: CIK=%s, title=%s", cik, title)

	facts, err := service.FetchCompanyFacts(ctx, cik)
	if err != nil {
		t.Fatalf("failed to fetch facts: %v", err)
	}

	statements, err := service.ExtractStatements(facts, "AMZN")
	if err != nil {
		t.Fatalf("failed to extract statements: %v", err)
	}

	t.Logf("Extracted %d statements for AMZN", len(statements))
	for _, s := range statements {
		if s.PeriodType == "ANNUAL" {
			t.Logf("Annual FY %d: Rev=%.1f, GP=%.1f, EBIT=%.1f, NI=%.1f, CapEx=%.1f, CFO=%.1f",
				s.FiscalYear, s.Revenue/1e6, s.GrossProfit/1e6, s.OperatingIncome/1e6, s.NetIncome/1e6, s.CapEx/1e6, s.OperatingCashFlow/1e6)
		}
	}
	if len(statements) < 3 {
		t.Errorf("expected at least 3 statements, got %d", len(statements))
	}
}
