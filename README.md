# Financial Terminal CLI (`finst`)

A high-performance, terminal-native, Bloomberg FA-style financial analysis CLI written in pure Go with zero CGO dependencies. `finst` queries, models, and renders a 7-period financial analysis table across historical actuals, LTM performance, analyst consensus, and forward heuristic projections using 100% free public data sources and local SQLite caching.

---

## Features

- **7-Period Financial Matrix**: Displays $T-3, T-2, T-1$ historical actuals, LTM/Base rolling metrics, and $T+1, T+2, T+3$ forward estimates.
- **Dual Ingestion Protocol**:
  - **SEC EDGAR XBRL REST API**: Primary high-fidelity reporting for US equities (10-K & 10-Q filings with CIK directory mapping).
  - **Public Global Feeds**: International coverage (`.PA`, `.T`, `.L`, `.TO`, `.DE`, etc.), live quotes, shares outstanding, and analyst consensus estimates.
- **Automated Forward Modeling**:
  - Automatically incorporates Wall Street consensus estimates ($T+1, T+2$).
  - Gracefully triggers a 3-year margin-based heuristic projection engine with clamped CAGR $[-5\%, +25\%]$ when consensus data is unavailable.
- **FX Normalization**: Live & historical cross-currency rate conversions powered by the ECB / Frankfurter API.
- **Zero-CGO SQLite Cache**: Local tiered caching (`~/.finst/cache.db`) for sub-second responses ($< 20\text{ms}$ on cache hit).
- **Multiple Views & Export**: Full Bloomberg slate/amber styled terminal table, compact summary view, raw JSON export, and CSV export.

---

## Installation & Setup

Requires **Go 1.22+**.

### 1. Build from Source

```bash
# Clone repository
git clone https://github.com/arisolta/finst.git
cd finst

# Build the executable
go build -o finst ./cmd/finst
```

### 2. Install to PATH (Run from Anywhere)

Choose one of the following methods to make `finst` globally accessible in your shell:

#### Option A: Install to User Binary Path (Recommended for macOS / Linux)
```bash
# Ensure ~/.local/bin exists
mkdir -p ~/.local/bin

# Copy executable
cp finst ~/.local/bin/finst
```
> *If `~/.local/bin` is not yet in your `$PATH`, add `export PATH="$HOME/.local/bin:$PATH"` to your `~/.zshrc` or `~/.bashrc`.*

#### Option B: Direct Go Install
```bash
go install ./cmd/finst
```
> *Installs `finst` directly to `$GOPATH/bin` (typically `~/go/bin`), which you can add to your `$PATH`.*

#### Option C: System-Wide (macOS / Linux)
```bash
sudo cp finst /usr/local/bin/finst
```

#### Option D: Windows (PowerShell)
```powershell
go build -o finst.exe .\cmd\finst
# Move finst.exe to a directory in your System PATH (e.g. C:\Windows\System32 or custom tools folder)
```

---

## Quick Start & Usage

Once in your `PATH`, run `finst` directly from any directory:

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

### 1. Current Spot Price Progression
In accordance with standard financial terminal matrix conventions (e.g. Bloomberg `FA`), the 7-period grid evaluates historical actuals, LTM, and forward consensus/projected estimates against the **current spot share price**:
- **Market Capitalization**: Calculated as $P_{\text{spot}} \times \text{Diluted Shares}_t$ for each period, capturing historical share buyback and dilution dynamics.
- **Valuation Multiples ($P/E, P/B, P/FCF, EV/EBITDA$)**: Display the progression curve and multiple compression/expansion on your **current entry price** as earnings grow from historical periods into the future ($T+1, T+2, T+3$).

### 2. Cash Flow De-Accumulation (LTM)
SEC Form 10-Q cash flow statements report cumulative Year-To-Date (YTD) amounts ($3\text{M}, 6\text{M}, 9\text{M}, 12\text{M}$). `finst` automatically de-accumulates discrete quarterly cash flows:
$$CF_{Q2} = \text{YTD}_{6\text{M}} - \text{YTD}_{3\text{M}}, \quad CF_{Q3} = \text{YTD}_{9\text{M}} - \text{YTD}_{6\text{M}}, \quad CF_{Q4} = \text{FY}_{12\text{M}} - \text{YTD}_{9\text{M}}$$
This ensures the LTM/Base period accurately reflects the true trailing 12-month rolling cash flow without single-quarter truncation.

### 3. Negative Equity & Deficit Standards
For companies with negative stockholders' equity resulting from leveraged recapitalizations or aggressive share buybacks (e.g. `DPZ`):
- **P/B & EV/Book**: Reported as `N/A` (economically undefined).
- **ROE**: Reported as `--` (Not Meaningful / avoids misleading negative returns for profitable businesses).
- **ROIC**: Evaluated on active invested capital ($\text{Total Debt} + \text{Equity} - \text{Cash}$).

---

## Testing

Run the full automated test suite covering metrics, multiple formulas, negative denominator handling, CAGR bounds clamping, SQLite repository TTLs, and table snapshot rendering:

```bash
go test -v ./...
```
