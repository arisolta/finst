package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/arisolta/finst/internal/api"
	"github.com/arisolta/finst/internal/cache"
	"github.com/arisolta/finst/internal/engine"
	"github.com/arisolta/finst/internal/model"
	"github.com/arisolta/finst/internal/ui"
)

const Version = "v1.0.4"

func main() {
	var (
		currencyFlag = flag.String("currency", "", "Standardize output currency (e.g. USD, EUR, JPY)")
		refreshFlag  = flag.Bool("refresh", false, "Bypass local SQLite cache and force-fetch fresh data")
		viewFlag     = flag.String("view", "standard", "Toggle between standard (full) and compact views")
		exportFlag   = flag.String("export", "", "Output raw parsed data as 'json' or 'csv'")
		dbPathFlag   = flag.String("db", "", "Custom SQLite cache database path")
		versionFlag  = flag.Bool("version", false, "Print version information")
		updateFlag   = flag.Bool("update", false, "Check for updates and self-update finst to the latest release")
		noColorFlag  = flag.Bool("no-color", false, "Disable ANSI color output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: finst <TICKER> [flags]\n")
		fmt.Fprintf(os.Stderr, "       finst update\n\n")
		fmt.Fprintf(os.Stderr, "Financial Terminal CLI (finst) — Bloomberg FA-style financial analysis\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  <TICKER>          Stock ticker (e.g. BSX, AAPL, MC.PA, 7203.T)\n")
		fmt.Fprintf(os.Stderr, "  update            Update finst to the latest release\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	var reorderedArgs []string
	var tickerArg string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "-") {
			reorderedArgs = append(reorderedArgs, arg)
			// If flag takes an argument without '='
			if (arg == "--currency" || arg == "-currency" || arg == "--view" || arg == "-view" ||
				arg == "--export" || arg == "-export" || arg == "--db" || arg == "-db") &&
				i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				i++
				reorderedArgs = append(reorderedArgs, os.Args[i])
			}
		} else {
			if tickerArg == "" {
				tickerArg = arg
			} else {
				reorderedArgs = append(reorderedArgs, arg)
			}
		}
	}
	if tickerArg != "" {
		reorderedArgs = append(reorderedArgs, tickerArg)
	}

	_ = flag.CommandLine.Parse(reorderedArgs)

	if *versionFlag {
		fmt.Printf("finst %s\n", Version)
		os.Exit(0)
	}

	if *noColorFlag {
		ui.SetColorsEnabled(false)
	}

	args := flag.Args()
	if *updateFlag || (len(args) > 0 && strings.ToLower(args[0]) == "update") {
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer updateCancel()
		if err := api.CheckAndSelfUpdate(updateCtx, Version); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	ticker := strings.ToUpper(strings.TrimSpace(args[len(args)-1]))
	if ticker == "" {
		fmt.Fprintln(os.Stderr, "Error: ticker symbol required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Initialize SQLite Cache
	db, err := cache.InitDB(*dbPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: SQLite cache initialization failed: %v\n", err)
	}
	var repo *cache.Repository
	if db != nil {
		defer db.Close()
		repo = cache.NewRepository(db)
	}

	// 2. Initialize Services
	httpClient := api.NewClient()
	edgarService := api.NewEdgarService(httpClient)
	yahooService := api.NewYahooService(httpClient)
	fxService := api.NewFXService(httpClient)
	datasetBuilder := engine.NewDatasetBuilder(fxService)

	// Check Cache
	var company *model.CompanyInfo
	var statements []model.FinancialStatement
	var estimates []model.ConsensusEstimate
	var price *model.PriceValuation

	cacheHit := false
	if repo != nil && !*refreshFlag {
		c, _ := repo.GetCompany(ctx, ticker)
		sts, stFresh, _ := repo.GetFinancialStatements(ctx, ticker)
		est, estFresh, _ := repo.GetConsensusEstimates(ctx, ticker)
		pv, pvFresh, _ := repo.GetPriceValuation(ctx, ticker)

		if c != nil && len(sts) >= 3 && stFresh && pv != nil && pvFresh {
			company = c
			statements = sts
			estimates = est
			price = pv
			_ = estFresh
			cacheHit = true
		}
	}

	// Ingestion Engine (Concurrent Goroutines)
	if !cacheHit {
		if repo != nil && *refreshFlag {
			_ = repo.InvalidateTicker(ctx, ticker)
		}

		var wg sync.WaitGroup
		var edgarStatements []model.FinancialStatement
		var yahooStatements []model.FinancialStatement
		var yahooCompany model.CompanyInfo
		var yahooPrice model.PriceValuation
		var yahooEstimates []model.ConsensusEstimate
		var isUSStock bool

		// Check if ticker is a US stock without exchange suffix
		if !strings.Contains(ticker, ".") {
			isUSStock = true
		}

		var histPrices []api.HistoricalPricePoint

		// Goroutine 1: SEC EDGAR (for US stocks)
		if isUSStock {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cik, title, err := edgarService.ResolveTicker(ctx, ticker)
				if err == nil && cik != "" {
					facts, fErr := edgarService.FetchCompanyFacts(ctx, cik)
					if fErr == nil && facts != nil {
						sts, pErr := edgarService.ExtractStatements(facts, ticker)
						if pErr == nil && len(sts) > 0 {
							edgarStatements = sts
							if company == nil {
								company = &model.CompanyInfo{
									Ticker:            ticker,
									CIK:               cik,
									Name:              title,
									Exchange:          "US",
									Currency:          "USD",
									ReportingStandard: "US-GAAP / SEC EDGAR",
									UpdatedAt:         time.Now(),
								}
							}
						}
					}
				}
			}()
		}

		// Goroutine 2: Yahoo Finance (Quotes, Profile, Consensus, and Global Timeseries Statements)
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, chart, err := yahooService.FetchQuoteSummary(ctx, ticker)
			if err == nil {
				yahooCompany = yahooService.ExtractCompanyInfo(ticker, res, chart)
				yahooPrice = yahooService.ExtractPriceValuation(ticker, res, chart)
				yahooStatements = yahooService.ExtractStatements(ticker, res)
				yahooEstimates = yahooService.ExtractConsensusEstimates(ticker, res, time.Now().Year()-1)
			}
			// Fetch full fundamental timeseries (provides Gross Profit, D&A, CapEx, Balance Sheet for international stocks)
			tsStatements, tsErr := yahooService.FetchFundamentalsTimeseries(ctx, ticker)
			if tsErr == nil && len(tsStatements) > 0 {
				if len(yahooStatements) == 0 {
					yahooStatements = tsStatements
				} else {
					// Merge or prefer timeseries statements if richer
					hasRichData := false
					for _, ts := range tsStatements {
						if ts.GrossProfit > 0 || ts.OperatingCashFlow != 0 || ts.TotalDebt > 0 {
							hasRichData = true
							break
						}
					}
					if hasRichData {
						yahooStatements = tsStatements
					}
				}
			}
		}()

		// Goroutine 3: Historical Monthly Prices (for Year-End Historical Market Caps)
		wg.Add(1)
		go func() {
			defer wg.Done()
			hp, hpErr := yahooService.FetchHistoricalMonthlyPrices(ctx, ticker)
			if hpErr == nil && len(hp) > 0 {
				histPrices = hp
			}
		}()

		wg.Wait()

		// Merge data sources
		if company == nil || company.Name == "" {
			company = &yahooCompany
		} else {
			if yahooCompany.Sector != "" {
				company.Sector = yahooCompany.Sector
			}
			if yahooCompany.Industry != "" {
				company.Industry = yahooCompany.Industry
			}
			if yahooCompany.Exchange != "" {
				company.Exchange = yahooCompany.Exchange
			}
		}

		price = &yahooPrice

		// Assign historical year-end close prices
		if len(histPrices) > 0 {
			for i := range edgarStatements {
				if edgarStatements[i].PeriodType == model.PeriodAnnual {
					edgarStatements[i].HistoricalPrice = api.FindClosestPrice(histPrices, edgarStatements[i].PeriodEndDate, edgarStatements[i].FiscalYear)
				}
			}
			for i := range yahooStatements {
				if yahooStatements[i].PeriodType == model.PeriodAnnual {
					yahooStatements[i].HistoricalPrice = api.FindClosestPrice(histPrices, yahooStatements[i].PeriodEndDate, yahooStatements[i].FiscalYear)
				}
			}
		}

		// Cross-enrich missing line items (e.g. Operating Income/EBIT, Gross Profit for fintechs/credit services, D&A, CapEx, Diluted Shares, Cash, Debt, Equity)
		for i := range edgarStatements {
			for _, ys := range yahooStatements {
				if ys.PeriodType == edgarStatements[i].PeriodType && ys.FiscalYear == edgarStatements[i].FiscalYear {
					if edgarStatements[i].OperatingIncome == 0 && ys.OperatingIncome != 0 {
						edgarStatements[i].OperatingIncome = ys.OperatingIncome
					}
					if edgarStatements[i].GrossProfit == 0 && ys.GrossProfit > 0 {
						edgarStatements[i].GrossProfit = ys.GrossProfit
					}
					if edgarStatements[i].CostOfRevenue == 0 && ys.CostOfRevenue > 0 {
						edgarStatements[i].CostOfRevenue = ys.CostOfRevenue
					}
					if edgarStatements[i].DepreciationAmortization == 0 && ys.DepreciationAmortization > 0 {
						edgarStatements[i].DepreciationAmortization = ys.DepreciationAmortization
					}
					if edgarStatements[i].OperatingCashFlow == 0 && ys.OperatingCashFlow != 0 {
						edgarStatements[i].OperatingCashFlow = ys.OperatingCashFlow
					}
					if edgarStatements[i].CapEx == 0 && ys.CapEx != 0 {
						edgarStatements[i].CapEx = ys.CapEx
					}
					if ys.DilutedShares > 0 {
						if edgarStatements[i].DilutedShares == 0 {
							edgarStatements[i].DilutedShares = ys.DilutedShares
						} else {
							// Detect split adjustments (e.g. 10:1 split where Yahoo is 24,940M and SEC was 2,494M)
							ratio := ys.DilutedShares / edgarStatements[i].DilutedShares
							if ratio > 1.4 || ratio < 0.7 {
								edgarStatements[i].DilutedShares = ys.DilutedShares
								if ys.AdjEPS != 0 {
									edgarStatements[i].AdjEPS = ys.AdjEPS
								} else if edgarStatements[i].NetIncome != 0 {
									edgarStatements[i].AdjEPS = edgarStatements[i].NetIncome / ys.DilutedShares
								}
							}
						}
					}
					if edgarStatements[i].AdjEPS == 0 && ys.AdjEPS != 0 {
						edgarStatements[i].AdjEPS = ys.AdjEPS
					}
					if edgarStatements[i].CashAndEquiv == 0 && ys.CashAndEquiv > 0 {
						edgarStatements[i].CashAndEquiv = ys.CashAndEquiv
					}
					if edgarStatements[i].TotalDebt == 0 && ys.TotalDebt > 0 {
						edgarStatements[i].TotalDebt = ys.TotalDebt
					}
					if edgarStatements[i].TotalEquity == 0 && ys.TotalEquity != 0 {
						edgarStatements[i].TotalEquity = ys.TotalEquity
					}
					if edgarStatements[i].CashDividendsPaid == 0 && ys.CashDividendsPaid > 0 {
						edgarStatements[i].CashDividendsPaid = ys.CashDividendsPaid
					}
					if edgarStatements[i].HistoricalPrice == 0 && ys.HistoricalPrice > 0 {
						edgarStatements[i].HistoricalPrice = ys.HistoricalPrice
					}
					break
				}
			}
		}

		// Prefer SEC EDGAR when 3+ complete annual statements are available, otherwise pick richer source
		var completeEdgarAnnuals int
		for _, s := range edgarStatements {
			if s.PeriodType == model.PeriodAnnual && s.Revenue > 0 {
				completeEdgarAnnuals++
			}
		}
		var completeYahooAnnuals int
		for _, s := range yahooStatements {
			if s.PeriodType == model.PeriodAnnual && s.Revenue > 0 {
				completeYahooAnnuals++
			}
		}

		if completeEdgarAnnuals >= 3 {
			statements = edgarStatements
		} else if completeYahooAnnuals >= completeEdgarAnnuals && completeYahooAnnuals > 0 {
			statements = yahooStatements
		} else if len(edgarStatements) > 0 {
			statements = edgarStatements
		} else {
			statements = yahooStatements
		}

		estimates = yahooEstimates

		if company.Ticker == "" {
			company.Ticker = ticker
		}
		if company.Name == "" {
			company.Name = ticker
		}
		if company.Currency == "" {
			company.Currency = "USD"
		}

		// Save to SQLite Cache
		if repo != nil {
			go func() {
				bgCtx, bgCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer bgCancel()
				if company != nil {
					_ = repo.SaveCompany(bgCtx, company)
				}
				if len(statements) > 0 {
					_ = repo.SaveFinancialStatements(bgCtx, statements)
				}
				if len(estimates) > 0 {
					_ = repo.SaveConsensusEstimates(bgCtx, estimates)
				}
				if price != nil {
					_ = repo.SavePriceValuation(bgCtx, price)
				}
			}()
		}
	}

	if len(statements) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no financial statement data could be found for ticker '%s'\n", ticker)
		os.Exit(1)
	}

	// Backfill historical prices if not yet cached on annual statements
	missingHistPrice := false
	for _, s := range statements {
		if s.PeriodType == model.PeriodAnnual && s.HistoricalPrice == 0 {
			missingHistPrice = true
			break
		}
	}
	if missingHistPrice {
		hp, hpErr := yahooService.FetchHistoricalMonthlyPrices(ctx, ticker)
		if hpErr == nil && len(hp) > 0 {
			for i := range statements {
				if statements[i].PeriodType == model.PeriodAnnual && statements[i].HistoricalPrice == 0 {
					statements[i].HistoricalPrice = api.FindClosestPrice(hp, statements[i].PeriodEndDate, statements[i].FiscalYear)
				}
			}
		}
	}

	// 3. Build Normalized Financial Dataset
	dataset, err := datasetBuilder.BuildDataset(ctx, *company, *price, statements, estimates, *currencyFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building financial dataset: %v\n", err)
		os.Exit(1)
	}

	// 4. Output Formatting
	switch strings.ToLower(*exportFlag) {
	case model.ExportJSON:
		jsonStr, err := ui.RenderJSON(dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(jsonStr)
	case model.ExportCSV:
		csvStr, err := ui.RenderCSV(dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting CSV: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(csvStr)
	default:
		screen := ui.RenderScreen(dataset, *viewFlag)
		fmt.Print(screen)
	}
}
