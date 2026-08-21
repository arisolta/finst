package cache

import (
	"context"
	"testing"
	"time"

	"finst/internal/model"
)

func TestSQLiteRepository(t *testing.T) {
	ctx := context.Background()
	// Use in-memory SQLite database
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init in-memory sqlite db: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)

	// 1. Test Company CRUD
	comp := &model.CompanyInfo{
		Ticker:            "BSX",
		CIK:               "0000885725",
		Name:              "Boston Scientific Corp",
		Exchange:          "NYSE",
		Sector:            "Healthcare",
		Industry:          "Medical Devices",
		Currency:          "USD",
		ReportingStandard: "US-GAAP / SEC EDGAR",
	}

	if err := repo.SaveCompany(ctx, comp); err != nil {
		t.Fatalf("failed to save company: %v", err)
	}

	fetchedComp, err := repo.GetCompany(ctx, "BSX")
	if err != nil || fetchedComp == nil {
		t.Fatalf("failed to fetch company: %v", err)
	}
	if fetchedComp.Name != "Boston Scientific Corp" || fetchedComp.CIK != "0000885725" {
		t.Errorf("unexpected company info: %+v", fetchedComp)
	}

	// 2. Test Statements CRUD and TTL
	statements := []model.FinancialStatement{
		{
			Ticker:        "BSX",
			PeriodType:    model.PeriodAnnual,
			FiscalYear:    2023,
			FiscalPeriod:  model.FiscalPeriodFY,
			PeriodEndDate: "2023-12-31",
			Revenue:       14240000000,
			GrossProfit:   10020000000,
			NetIncome:     2450000000,
		},
		{
			Ticker:        "BSX",
			PeriodType:    model.PeriodAnnual,
			FiscalYear:    2024,
			FiscalPeriod:  model.FiscalPeriodFY,
			PeriodEndDate: "2024-12-31",
			Revenue:       16740000000,
			GrossProfit:   11850000000,
			NetIncome:     3010000000,
		},
		{
			Ticker:        "BSX",
			PeriodType:    model.PeriodAnnual,
			FiscalYear:    2025,
			FiscalPeriod:  model.FiscalPeriodFY,
			PeriodEndDate: "2025-12-31",
			Revenue:       20074000000,
			GrossProfit:   14174000000,
			NetIncome:     3606000000,
		},
	}

	if err := repo.SaveFinancialStatements(ctx, statements); err != nil {
		t.Fatalf("failed to save statements: %v", err)
	}

	fetchedSts, isFresh, err := repo.GetFinancialStatements(ctx, "BSX")
	if err != nil || len(fetchedSts) != 3 || !isFresh {
		t.Fatalf("expected 3 fresh statements, got len=%d, isFresh=%v, err=%v", len(fetchedSts), isFresh, err)
	}

	// 3. Test Price Cache
	pv := &model.PriceValuation{
		Ticker:            "BSX",
		SharePrice:        82.40,
		SharesOutstanding: 1473900000,
		MarketCap:         121450000000,
		EnterpriseValue:   131885000000,
		Currency:          "USD",
		UpdatedAt:         time.Now(),
	}

	if err := repo.SavePriceValuation(ctx, pv); err != nil {
		t.Fatalf("failed to save price: %v", err)
	}

	fetchedPV, pvFresh, err := repo.GetPriceValuation(ctx, "BSX")
	if err != nil || fetchedPV == nil || !pvFresh {
		t.Fatalf("expected fresh price valuation, got %+v, isFresh=%v, err=%v", fetchedPV, pvFresh, err)
	}

	// 4. Test Invalidate Ticker (force refresh)
	if err := repo.InvalidateTicker(ctx, "BSX"); err != nil {
		t.Fatalf("failed to invalidate ticker: %v", err)
	}

	afterComp, _ := repo.GetCompany(ctx, "BSX")
	if afterComp != nil {
		t.Errorf("expected company to be deleted after invalidation, got %+v", afterComp)
	}
	afterSts, _, _ := repo.GetFinancialStatements(ctx, "BSX")
	if len(afterSts) != 0 {
		t.Errorf("expected 0 statements after invalidation, got %d", len(afterSts))
	}
}
