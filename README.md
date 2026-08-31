# Financial Terminal CLI (`finst`)

A high-performance, terminal-native, Bloomberg FA-style financial analysis CLI written in pure Go with zero CGO dependencies. `finst` queries, models, and renders a 7-period financial analysis table across historical actuals, LTM performance, analyst consensus, and forward heuristic projections using 100% free public data sources and local SQLite caching.

---

## Features

- **6-Period Financial Matrix**: Displays `T-3`, `T-2`, `T-1` historical actuals, `LTM/Base` rolling metrics, and `T+1`, `T+2` forward consensus estimates (or synthetic heuristic projections if uncovered).
- **Dual Ingestion Protocol**:
  - **SEC EDGAR XBRL REST API**: Primary high-fidelity reporting for US equities (10-K & 10-Q filings with CIK directory mapping).
  - **Public Global Feeds**: International coverage (`.PA`, `.T`, `.L`, `.TO`, `.DE`, etc.), live quotes, shares outstanding, and analyst consensus estimates.
- **Automated Forward Modeling**:
  - Automatically incorporates Wall Street consensus estimates (`T+1`, `T+2`).
  - Gracefully triggers a 3-year margin-based heuristic projection engine with clamped CAGR `[-5%, +25%]` when consensus data is unavailable.
- **FX Normalization**: Live & historical cross-currency rate conversions powered by the ECB / Frankfurter API.
- **Zero-CGO SQLite Cache**: Local tiered caching (`~/.finst/cache.db`) for sub-second responses (`< 20ms` on cache hit).
- **Multiple Views & Export**: Full Bloomberg slate/amber styled terminal table, compact summary view, raw JSON export, and CSV export.

---

## Installation

### 1. One-Line Install (No Go Required)

#### macOS & Linux (Apple Silicon M1/M2/M3/M4, Intel, Linux x86/ARM)
Open your terminal and run:
```bash
curl -fsSL https://raw.githubusercontent.com/arisolta/finst/main/install.sh | sh
```

#### Windows (PowerShell)
Open PowerShell and run:
```powershell
irm https://raw.githubusercontent.com/arisolta/finst/main/install.ps1 | iex
```

---

### 2. Updating `finst`
Once installed, `finst` can update itself directly:
```bash
finst update
```

---

### 3. Build from Source (Developers)

Requires **Go 1.22+**:
```bash
# Clone and build
git clone https://github.com/arisolta/finst.git
cd finst
go build -o finst ./cmd/finst

# Move to your local PATH
mkdir -p ~/.local/bin && cp finst ~/.local/bin/finst
```

---

## Quick Start & Usage

Run `finst` directly from any terminal:

```bash
# US Equities (audited SEC EDGAR XBRL data)
finst AAPL
finst MSFT
finst BSX
finst NVDA
finst DPZ

# Global Equities (with exchange suffix)
finst MC.PA          # Euronext Paris (LVMH)
finst NESN.SW        # SIX Swiss Exchange (Nestlé)
finst 0700.HK        # Hong Kong Stock Exchange (Tencent)
finst 7203.T         # Tokyo Stock Exchange (Toyota)
finst AZN.L          # London Stock Exchange (AstraZeneca)
finst HEN3.DE        # XETRA (Henkel)

# Currency Conversion (ECB / Frankfurter rates)
finst NESN.SW --currency USD
finst 0700.HK --currency USD

# Alternate Views & Formats
finst AAPL --view compact     # Condensed summary view with key multiples
finst MSFT --export json      # Structured JSON output
finst NVDA --export csv       # CSV table export
finst BSX --refresh           # Bypass cache and force fresh data fetch
finst update                  # Self-update to latest release
finst --version               # Display CLI version
```

---

## System Architecture

```text
                                  [ CLI Invocation: finst <TICKER> ]
                                                   │
                                                   ▼
                                     ┌───────────────────────────┐
                                     │   Ticker Resolution &     │
                                     │     Config Manager        │
                                     └─────────────┬─────────────┘
                                                   │
                                                   ▼
                                     ┌───────────────────────────┐
                                     │  SQLite Caching Engine    │
                                     │    (~/.finst/cache.db)    │
                                     └──────┬─────────────▲──────┘
                                            │             │
                             Cache Miss /   │             │ Write fresh data
                             Expired TTL    │             │ to cache
                                            ▼             │
┌─────────────────────────────────────────────────────────┴───────────────────────────────────────────────────────┐
│ Concurrent Ingestion Engine (Go Goroutines + Worker Pool)                                                       │
│                                                                                                                 │
│   ┌──────────────────────────┐    ┌──────────────────────────┐    ┌─────────────────────────────────────────┐   │
│   │ 1. SEC EDGAR API         │    │ 2. Public Global Feeds   │    │ 3. Open FX Rates API                    │   │
│   │ (US Stocks - 10-K / 10-Q)│    │ (Yahoo Finance / Global) │    │ (Frankfurter / ECB Open API)            │   │
│   │ - 10-Digit Padded CIK    │    │ - Global Statements      │    │ - Spot Exchange Rates                   │   │
│   │ - Strict User-Agent Req. │    │ - Consensus Estimates    │    │ - Historical Cross-Currency Rates       │   │
│   │ - Raw XBRL Facts JSON    │    │ - Quote / Shares Out     │    │                                         │   │
│   └──────────────────────────┘    └──────────────────────────┘    └─────────────────────────────────────────┘   │
└───────────────────────────────────────────┬─────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
                             ┌─────────────────────────────┐
                             │ Financial Processing Engine │
                             │ - Harmonize IFRS / US-GAAP  │
                             │ - Compute LTM Sums          │
                             │ - 3-Year Fallback Forecaster│
                             │ - Ratio & Valuation Math    │
                             └──────────────┬──────────────┘
                                            │
                                            ▼
                             ┌─────────────────────────────┐
                             │ Terminal Presentation Grid  │
                             │ (ANSI styling, Box-drawing) │
                             └─────────────────────────────┘
```

---

## Valuation & Modeling Methodology

### 1. Point-in-Time Market Cap & Spot Multiples Progression
- **Historical Periods (`T-3`, `T-2`, `T-1`)**: Market Capitalization and valuation multiples (`P/E`, `P/B`, `P/FCF`, `EV/Sales`, `EV/EBITDA`, `EV/EBIT`, `Dividend Yield %`) are evaluated at the **actual historical share price at the end of each respective fiscal year**, reflecting the true point-in-time valuation multiples the company traded at.
- **`LTM/Base`**: Evaluated at **today's live spot share price**, displaying the company's current trailing multiple and dividend yield.
- **Forward Estimates (`T+1`, `T+2`)**: Multiples display the forward multiple compression/expansion and projected dividend yield on your **current entry price** as projected earnings and dividends grow.

### 2. Cash Flow De-Accumulation (LTM)
SEC Form 10-Q cash flow statements report cumulative Year-To-Date (YTD) amounts (`3M`, `6M`, `9M`, `12M`). `finst` automatically de-accumulates discrete quarterly cash flows:

- `Q1 = YTD(3M)`
- `Q2 = YTD(6M) - YTD(3M)`
- `Q3 = YTD(9M) - YTD(6M)`
- `Q4 = FY(12M) - YTD(9M)`

This ensures the `LTM/Base` period accurately reflects the true trailing 12-month rolling cash flow without single-quarter truncation.

### 3. Negative Equity & Deficit Standards
For companies with negative stockholders' equity resulting from leveraged recapitalizations or aggressive share buybacks (e.g. `DPZ`):
- **P/B**: Reported as `N/A` (economically undefined).
- **ROE**: Reported as `--` (Not Meaningful / avoids misleading negative returns for profitable businesses).
- **ROIC**: Evaluated on active invested capital (`Total Debt + Total Equity - Cash`).

### 4. Forward Estimates & Hybrid Forecasting Engine
The forward matrix (`T+1`, `T+2`) uses a high-fidelity consensus model with automated synthetic fallback:
- **Analyst Consensus (`Cons`)**: Ingested directly from institutional sell-side equity research consensus (covering mean Revenue and EPS forecasts for `T+1` and `T+2`). Intermediate statement lines (Gross Profit, EBITDA, D&A, CapEx, and FCF) are modeled by applying the company's time-weighted historical margins to the consensus top-line numbers.
- **Synthetic Fallback Projections (`Proj`)**: When a ticker has zero sell-side coverage, `finst` automatically generates conservative synthetic forecasts:
  - **Revenue Growth**: Clamped historical CAGR (`[-5.0%, +25.0%]`).
  - **Operating Margins**: Time-weighted historical baseline (`60% × T-1 + 25% × T-2 + 15% × T-3`).

---

## Testing

Run the full automated test suite covering metrics, multiple formulas, negative denominator handling, CAGR bounds clamping, SQLite repository TTLs, and table snapshot rendering:

```bash
go test -v ./...
```
