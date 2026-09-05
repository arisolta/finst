package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arisolta/finst/internal/model"
)

type YahooService struct {
	client    *Client
	crumb     string
	crumbLock sync.RWMutex
}

func NewYahooService(client *Client) *YahooService {
	return &YahooService{
		client: client,
	}
}

// EnsureCrumb retrieves a session cookie and crumb from Yahoo Finance.
func (s *YahooService) EnsureCrumb(ctx context.Context) (string, error) {
	s.crumbLock.Lock()
	defer s.crumbLock.Unlock()

	if s.crumb != "" {
		return s.crumb, nil
	}

	headers := map[string]string{
		"User-Agent":      WebUserAgent,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.5",
	}

	// 1. Visit fc.yahoo.com to obtain cookies
	// Note: fc.yahoo.com returns 404 with cookies set; our client or http ignores 404 if cookies are captured
	req, err := httpNewGetRequest(ctx, "https://fc.yahoo.com", headers)
	if err == nil {
		resp, rErr := s.client.HTTPClient.Do(req)
		if rErr == nil {
			resp.Body.Close()
		}
	}

	// 2. Fetch crumb
	crumbHeaders := map[string]string{
		"User-Agent":      WebUserAgent,
		"Accept":          "*/*",
		"Accept-Language": "en-US,en;q=0.5",
		"Referer":         "https://finance.yahoo.com/",
		"Origin":          "https://finance.yahoo.com",
	}
	opts2 := &RequestOptions{
		Headers: crumbHeaders,
		Timeout: 10 * time.Second,
		Retries: 2,
	}

	crumbData, err := s.client.Get(ctx, "https://query2.finance.yahoo.com/v1/test/getcrumb", opts2)
	if err != nil {
		// Try query1 as fallback
		crumbData, err = s.client.Get(ctx, "https://query1.finance.yahoo.com/v1/test/getcrumb", opts2)
	}

	if err != nil {
		return "", fmt.Errorf("failed to retrieve Yahoo crumb: %w", err)
	}

	crumbStr := strings.TrimSpace(string(crumbData))
	if crumbStr == "" || strings.Contains(crumbStr, "Too Many Requests") || strings.Contains(crumbStr, "<") {
		return "", fmt.Errorf("invalid crumb received from Yahoo: %s", crumbStr)
	}

	s.crumb = crumbStr
	return s.crumb, nil
}

// Yahoo raw JSON structures
type YahooQuoteSummaryEnvelope struct {
	QuoteSummary struct {
		Result []YahooQuoteSummaryResult `json:"result"`
		Error  *YahooAPIError            `json:"error"`
	} `json:"quoteSummary"`
}

type YahooAPIError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type YahooQuoteSummaryResult struct {
	AssetProfile                     YahooAssetProfile                     `json:"assetProfile"`
	FinancialData                    YahooFinancialData                    `json:"financialData"`
	DefaultKeyStatistics             YahooDefaultKeyStatistics             `json:"defaultKeyStatistics"`
	IncomeStatementHistory           YahooIncomeStatementHistory           `json:"incomeStatementHistory"`
	BalanceSheetHistory              YahooBalanceSheetHistory              `json:"balanceSheetHistory"`
	CashflowStatementHistory         YahooCashflowStatementHistory         `json:"cashflowStatementHistory"`
	IncomeStatementHistoryQuarterly  YahooIncomeStatementHistoryQuarterly  `json:"incomeStatementHistoryQuarterly"`
	CashflowStatementHistoryQuarterly YahooCashflowStatementHistoryQuarterly `json:"cashflowStatementHistoryQuarterly"`
	EarningsTrend                    YahooEarningsTrend                    `json:"earningsTrend"`
}

type YahooRawFmt struct {
	Raw     float64 `json:"raw"`
	Fmt     string  `json:"fmt"`
	LongFmt string  `json:"longFmt"`
}

type YahooAssetProfile struct {
	Sector   string `json:"sector"`
	Industry string `json:"industry"`
}

type YahooFinancialData struct {
	CurrentPrice        YahooRawFmt `json:"currentPrice"`
	EnterpriseValue     YahooRawFmt `json:"enterpriseValue"`
	FinancialCurrency   string      `json:"financialCurrency"`
	TotalRevenue        YahooRawFmt `json:"totalRevenue"`
	GrossMargins        YahooRawFmt `json:"grossMargins"`
	EbitdaMargins       YahooRawFmt `json:"ebitdaMargins"`
	OperatingMargins    YahooRawFmt `json:"operatingMargins"`
	ProfitMargins       YahooRawFmt `json:"profitMargins"`
	ReturnOnEquity      YahooRawFmt `json:"returnOnEquity"`
	OperatingCashflow   YahooRawFmt `json:"operatingCashflow"`
	FreeCashflow        YahooRawFmt `json:"freeCashflow"`
	TotalDebt           YahooRawFmt `json:"totalDebt"`
	TotalCash           YahooRawFmt `json:"totalCash"`
}

type YahooDefaultKeyStatistics struct {
	SharesOutstanding YahooRawFmt `json:"sharesOutstanding"`
	EnterpriseValue   YahooRawFmt `json:"enterpriseValue"`
	PriceToBook       YahooRawFmt `json:"priceToBook"`
	ForwardPE         YahooRawFmt `json:"forwardPE"`
	TrailingPE        YahooRawFmt `json:"trailingPE"`
}

type YahooIncomeStatementHistory struct {
	IncomeStatementHistory []YahooIncomeStatement `json:"incomeStatementHistory"`
}

type YahooIncomeStatementHistoryQuarterly struct {
	IncomeStatementHistory []YahooIncomeStatement `json:"incomeStatementHistory"`
}

type YahooIncomeStatement struct {
	EndDate                  YahooRawFmt `json:"endDate"`
	TotalRevenue             YahooRawFmt `json:"totalRevenue"`
	CostOfRevenue            YahooRawFmt `json:"costOfRevenue"`
	GrossProfit              YahooRawFmt `json:"grossProfit"`
	OperatingIncome          YahooRawFmt `json:"operatingIncome"`
	NetIncome                YahooRawFmt `json:"netIncome"`
	ResearchDevelopment      YahooRawFmt `json:"researchDevelopment"`
	SellingGeneralAdministrative YahooRawFmt `json:"sellingGeneralAdministrative"`
	TotalOperatingExpenses   YahooRawFmt `json:"totalOperatingExpenses"`
}

type YahooBalanceSheetHistory struct {
	BalanceSheetStatements []YahooBalanceSheet `json:"balanceSheetStatements"`
}

type YahooBalanceSheet struct {
	EndDate                  YahooRawFmt `json:"endDate"`
	Cash                     YahooRawFmt `json:"cash"`
	ShortTermInvestments     YahooRawFmt `json:"shortTermInvestments"`
	CashAndCashEquivalents   YahooRawFmt `json:"cashAndCashEquivalents"`
	LongTermDebt             YahooRawFmt `json:"longTermDebt"`
	ShortLongTermDebt        YahooRawFmt `json:"shortLongTermDebt"`
	TotalStockholderEquity   YahooRawFmt `json:"totalStockholderEquity"`
	PreferredStock           YahooRawFmt `json:"preferredStock"`
}

type YahooCashflowStatementHistory struct {
	CashflowStatements []YahooCashflowStatement `json:"cashflowStatements"`
}

type YahooCashflowStatementHistoryQuarterly struct {
	CashflowStatements []YahooCashflowStatement `json:"cashflowStatements"`
}

type YahooCashflowStatement struct {
	EndDate                     YahooRawFmt `json:"endDate"`
	TotalCashFromOperatingActivities YahooRawFmt `json:"totalCashFromOperatingActivities"`
	CapitalExpenditures         YahooRawFmt `json:"capitalExpenditures"`
	Depreciation                YahooRawFmt `json:"depreciation"`
}

type YahooEarningsTrend struct {
	Trend []YahooTrendItem `json:"trend"`
}

type YahooTrendItem struct {
	Period           string `json:"period"` // "0q", "+1q", "0y", "+1y", "+2y"
	EndDate          string `json:"endDate"`
	Growth           YahooRawFmt `json:"growth"`
	EarningsEstimate struct {
		Avg             YahooRawFmt `json:"avg"`
		Low             YahooRawFmt `json:"low"`
		High            YahooRawFmt `json:"high"`
		NumberOfAnalysts YahooRawFmt `json:"numberOfAnalysts"`
	} `json:"earningsEstimate"`
	RevenueEstimate struct {
		Avg             YahooRawFmt `json:"avg"`
		Low             YahooRawFmt `json:"low"`
		High            YahooRawFmt `json:"high"`
		NumberOfAnalysts YahooRawFmt `json:"numberOfAnalysts"`
		Growth          YahooRawFmt `json:"growth"`
	} `json:"revenueEstimate"`
}

// Chart response for fallback metadata / price
type YahooChartEnvelope struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				ExchangeName       string  `json:"exchangeName"`
				FullExchangeName   string  `json:"fullExchangeName"`
				InstrumentType     string  `json:"instrumentType"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				FiftyTwoWeekHigh   float64 `json:"fiftyTwoWeekHigh"`
				FiftyTwoWeekLow    float64 `json:"fiftyTwoWeekLow"`
				LongName           string  `json:"longName"`
				ShortName          string  `json:"shortName"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

// FetchQuoteSummary fetches all financial statements, valuation, company profile, and consensus estimates.
func (s *YahooService) FetchQuoteSummary(ctx context.Context, ticker string) (*YahooQuoteSummaryResult, *YahooChartEnvelope, error) {
	crumb, err := s.EnsureCrumb(ctx)
	if err != nil {
		// Log or proceed without crumb to try chart endpoint
	}

	modules := "financialData,defaultKeyStatistics,assetProfile,incomeStatementHistory,balanceSheetHistory,cashflowStatementHistory,incomeStatementHistoryQuarterly,cashflowStatementHistoryQuarterly,earningsTrend"
	summaryURL := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?crumb=%s&modules=%s",
		url.PathEscape(ticker), url.QueryEscape(crumb), modules)

	headers := map[string]string{
		"User-Agent":      WebUserAgent,
		"Accept":          "*/*",
		"Accept-Language": "en-US,en;q=0.5",
		"Referer":         "https://finance.yahoo.com/",
	}

	opts := &RequestOptions{
		Headers: headers,
		Timeout: 15 * time.Second,
		Retries: 2,
	}

	var summaryResult *YahooQuoteSummaryResult
	data, sErr := s.client.Get(ctx, summaryURL, opts)
	if sErr == nil {
		var env YahooQuoteSummaryEnvelope
		if jsonErr := json.Unmarshal(data, &env); jsonErr == nil && len(env.QuoteSummary.Result) > 0 {
			summaryResult = &env.QuoteSummary.Result[0]
		}
	}

	// Also fetch chart for comprehensive metadata (names, currency, exchange, spot price)
	chartURL := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", url.PathEscape(ticker))
	var chartEnv *YahooChartEnvelope
	chartData, cErr := s.client.Get(ctx, chartURL, opts)
	if cErr == nil {
		var c YahooChartEnvelope
		if jsonErr := json.Unmarshal(chartData, &c); jsonErr == nil && len(c.Chart.Result) > 0 {
			chartEnv = &c
		}
	}

	if summaryResult == nil && chartEnv == nil {
		return nil, nil, fmt.Errorf("failed to fetch data from Yahoo Finance for ticker %s (summaryErr: %v, chartErr: %v)", ticker, sErr, cErr)
	}

	return summaryResult, chartEnv, nil
}

// ExtractCompanyInfo builds model.CompanyInfo from Yahoo data.
func (s *YahooService) ExtractCompanyInfo(ticker string, res *YahooQuoteSummaryResult, chart *YahooChartEnvelope) model.CompanyInfo {
	info := model.CompanyInfo{
		Ticker:    strings.ToUpper(ticker),
		Exchange:  "Global",
		Currency:  "USD",
		UpdatedAt: time.Now(),
	}

	if chart != nil && len(chart.Chart.Result) > 0 {
		m := chart.Chart.Result[0].Meta
		if m.LongName != "" {
			info.Name = m.LongName
		} else if m.ShortName != "" {
			info.Name = m.ShortName
		}
		if m.FullExchangeName != "" {
			info.Exchange = m.FullExchangeName
		} else if m.ExchangeName != "" {
			info.Exchange = m.ExchangeName
		}
		if m.Currency != "" {
			info.Currency = strings.ToUpper(m.Currency)
		}
	}

	if res != nil {
		if res.AssetProfile.Sector != "" {
			info.Sector = res.AssetProfile.Sector
		}
		if res.AssetProfile.Industry != "" {
			info.Industry = res.AssetProfile.Industry
		}
		if info.Name == "" {
			info.Name = ticker
		}
		if res.FinancialData.FinancialCurrency != "" {
			info.Currency = strings.ToUpper(res.FinancialData.FinancialCurrency)
		}
	}

	if info.Name == "" {
		info.Name = ticker
	}
	return info
}

// ExtractPriceValuation extracts live market data.
func (s *YahooService) ExtractPriceValuation(ticker string, res *YahooQuoteSummaryResult, chart *YahooChartEnvelope) model.PriceValuation {
	pv := model.PriceValuation{
		Ticker:    strings.ToUpper(ticker),
		Currency:  "USD",
		UpdatedAt: time.Now(),
	}

	isPence := false
	if chart != nil && len(chart.Chart.Result) > 0 {
		m := chart.Chart.Result[0].Meta
		if m.Currency == "GBp" || m.Currency == "GBX" || m.Currency == "ILA" || m.Currency == "ZAc" {
			isPence = true
		}
		pv.SharePrice = m.RegularMarketPrice
		if m.Currency != "" {
			pv.Currency = strings.ToUpper(m.Currency)
		}
	}

	if res != nil {
		if res.FinancialData.CurrentPrice.Raw > 0 {
			pv.SharePrice = res.FinancialData.CurrentPrice.Raw
		}
		if res.DefaultKeyStatistics.SharesOutstanding.Raw > 0 {
			pv.SharesOutstanding = res.DefaultKeyStatistics.SharesOutstanding.Raw
		}
		if res.FinancialData.EnterpriseValue.Raw > 0 {
			pv.EnterpriseValue = res.FinancialData.EnterpriseValue.Raw
		} else if res.DefaultKeyStatistics.EnterpriseValue.Raw > 0 {
			pv.EnterpriseValue = res.DefaultKeyStatistics.EnterpriseValue.Raw
		}
	}

	if isPence || (pv.Currency == "GBP" && pv.SharePrice > 1000) {
		pv.SharePrice /= 100.0
		pv.Currency = "GBP"
	}

	if pv.SharesOutstanding > 0 && pv.SharePrice > 0 && pv.MarketCap == 0 {
		pv.MarketCap = pv.SharesOutstanding * pv.SharePrice
	}

	return pv
}

// YahooTimeseriesEnvelope represents response from fundamentals-timeseries endpoint
type YahooTimeseriesEnvelope struct {
	Timeseries struct {
		Result []map[string]any `json:"result"`
		Error  any              `json:"error"`
	} `json:"timeseries"`
}

// FetchFundamentalsTimeseries retrieves comprehensive historical statements using the modern timeseries API.
func (s *YahooService) FetchFundamentalsTimeseries(ctx context.Context, ticker string) ([]model.FinancialStatement, error) {
	crumb, err := s.EnsureCrumb(ctx)
	if err != nil {
		return nil, err
	}

	types := []string{
		"annualTotalRevenue", "annualCostOfRevenue", "annualGrossProfit", "annualOperatingIncome",
		"annualOperatingExpense", "annualNetIncomeContinuousOperations", "annualNetIncome", "annualDilutedEPS",
		"annualOperatingCashFlow", "annualCapitalExpenditure", "annualEndCashPosition", "annualCashAndCashEquivalents",
		"annualTotalDebt", "annualTotalStockholderEquity", "annualStockholdersEquity", "annualReconciledDepreciation",
		"annualNormalizedEBITDA", "annualEBITDA", "annualFreeCashFlow", "annualPreferredStock",
		"annualDilutedAverageShares", "annualOrdinarySharesNumber", "annualBasicAverageShares",
		"annualCashDividendsPaid",
		"trailingTotalRevenue", "trailingCostOfRevenue", "trailingGrossProfit", "trailingOperatingIncome",
		"trailingOperatingExpense", "trailingNetIncomeContinuousOperations", "trailingNetIncome", "trailingDilutedEPS",
		"trailingOperatingCashFlow", "trailingCapitalExpenditure", "trailingEndCashPosition", "trailingCashAndCashEquivalents",
		"trailingTotalDebt", "trailingTotalStockholderEquity", "trailingStockholdersEquity", "trailingReconciledDepreciation",
		"trailingNormalizedEBITDA", "trailingEBITDA", "trailingFreeCashFlow",
		"trailingDilutedAverageShares", "trailingOrdinarySharesNumber",
		"trailingCashDividendsPaid",
		"quarterlyTotalRevenue", "quarterlyCostOfRevenue", "quarterlyGrossProfit", "quarterlyOperatingIncome",
		"quarterlyNetIncomeContinuousOperations", "quarterlyNetIncome", "quarterlyOperatingCashFlow", "quarterlyCapitalExpenditure",
		"quarterlyReconciledDepreciation", "quarterlyDilutedEPS", "quarterlyEndCashPosition", "quarterlyTotalDebt",
		"quarterlyTotalStockholderEquity", "quarterlyDilutedAverageShares", "quarterlyOrdinarySharesNumber",
		"quarterlyCashDividendsPaid",
	}

	typeStr := strings.Join(types, ",")
	tsURL := fmt.Sprintf("https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/%s?crumb=%s&period1=1577836800&period2=1893456000&type=%s&merge=false",
		url.PathEscape(ticker), url.QueryEscape(crumb), typeStr)

	headers := map[string]string{
		"User-Agent":      WebUserAgent,
		"Accept":          "*/*",
		"Accept-Language": "en-US,en;q=0.5",
		"Referer":         "https://finance.yahoo.com/",
	}

	opts := &RequestOptions{
		Headers: headers,
		Timeout: 15 * time.Second,
		Retries: 2,
	}

	data, err := s.client.Get(ctx, tsURL, opts)
	if err != nil {
		return nil, err
	}

	var env YahooTimeseriesEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}

	type StKey struct {
		PeriodType string
		DateStr    string
	}
	stMap := make(map[StKey]*model.FinancialStatement)

	getOrCreate := func(pType, dateStr string) *model.FinancialStatement {
		k := StKey{PeriodType: pType, DateStr: dateStr}
		if st, ok := stMap[k]; ok {
			return st
		}
		var fy int
		var fp string
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			fy = t.Year()
			if pType == model.PeriodAnnual {
				fp = model.FiscalPeriodFY
			} else {
				month := int(t.Month())
				if month <= 3 {
					fp = "Q1"
				} else if month <= 6 {
					fp = "Q2"
				} else if month <= 9 {
					fp = "Q3"
				} else {
					fp = "Q4"
				}
			}
		}

		st := &model.FinancialStatement{
			Ticker:        ticker,
			PeriodType:    pType,
			FiscalYear:    fy,
			FiscalPeriod:  fp,
			PeriodEndDate: dateStr,
			UpdatedAt:     time.Now(),
		}
		stMap[k] = st
		return st
	}

	for _, item := range env.Timeseries.Result {
		metaObj, ok := item["meta"].(map[string]any)
		if !ok {
			continue
		}
		typeArr, ok := metaObj["type"].([]any)
		if !ok || len(typeArr) == 0 {
			continue
		}
		metricName, _ := typeArr[0].(string)
		if metricName == "" {
			continue
		}

		valsArr, ok := item[metricName].([]any)
		if !ok {
			continue
		}

		pType := model.PeriodAnnual
		if strings.HasPrefix(metricName, "quarterly") {
			pType = model.PeriodQuarterly
		} else if strings.HasPrefix(metricName, "trailing") {
			pType = "LTM"
		}

		for _, valItem := range valsArr {
			vMap, ok := valItem.(map[string]any)
			if !ok {
				continue
			}
			dateStr, _ := vMap["asOfDate"].(string)
			if dateStr == "" {
				continue
			}
			repVal, ok := vMap["reportedValue"].(map[string]any)
			if !ok {
				continue
			}
			numVal, ok := repVal["raw"].(float64)
			if !ok {
				continue
			}

			st := getOrCreate(pType, dateStr)
			switch metricName {
			case "annualTotalRevenue", "quarterlyTotalRevenue", "trailingTotalRevenue":
				st.Revenue = numVal
			case "annualCostOfRevenue", "quarterlyCostOfRevenue", "trailingCostOfRevenue":
				st.CostOfRevenue = numVal
			case "annualGrossProfit", "quarterlyGrossProfit", "trailingGrossProfit":
				st.GrossProfit = numVal
			case "annualOperatingIncome", "quarterlyOperatingIncome", "trailingOperatingIncome":
				st.OperatingIncome = numVal
			case "annualReconciledDepreciation", "quarterlyReconciledDepreciation", "trailingReconciledDepreciation":
				st.DepreciationAmortization = numVal
			case "annualNetIncomeContinuousOperations", "annualNetIncome", "quarterlyNetIncomeContinuousOperations", "quarterlyNetIncome", "trailingNetIncomeContinuousOperations", "trailingNetIncome":
				st.NetIncome = numVal
			case "annualDilutedEPS", "quarterlyDilutedEPS", "trailingDilutedEPS":
				st.AdjEPS = numVal
			case "annualOperatingCashFlow", "quarterlyOperatingCashFlow", "trailingOperatingCashFlow":
				st.OperatingCashFlow = numVal
			case "annualCapitalExpenditure", "quarterlyCapitalExpenditure", "trailingCapitalExpenditure":
				st.CapEx = numVal
			case "annualEndCashPosition", "annualCashAndCashEquivalents", "quarterlyEndCashPosition", "trailingEndCashPosition":
				st.CashAndEquiv = numVal
			case "annualTotalDebt", "quarterlyTotalDebt", "trailingTotalDebt":
				st.TotalDebt = numVal
			case "annualPreferredStock":
				st.PreferredStock = numVal
			case "annualCashDividendsPaid", "quarterlyCashDividendsPaid", "trailingCashDividendsPaid":
				st.CashDividendsPaid = math.Abs(numVal)
			case "annualTotalStockholderEquity", "annualStockholdersEquity", "quarterlyTotalStockholderEquity", "trailingTotalStockholderEquity":
				st.TotalEquity = numVal
			case "annualDilutedAverageShares", "annualOrdinarySharesNumber", "annualBasicAverageShares",
				"trailingDilutedAverageShares", "trailingOrdinarySharesNumber",
				"quarterlyDilutedAverageShares", "quarterlyOrdinarySharesNumber":
				if st.DilutedShares == 0 || metricName == "annualDilutedAverageShares" {
					st.DilutedShares = numVal
				}
			}
		}
	}

	var results []model.FinancialStatement
	for _, st := range stMap {
		if st.Revenue == 0 && st.NetIncome == 0 && st.TotalEquity == 0 {
			continue
		}
		if st.GrossProfit == 0 && st.Revenue > 0 && st.CostOfRevenue > 0 {
			st.GrossProfit = st.Revenue - st.CostOfRevenue
		}
		results = append(results, *st)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].FiscalYear != results[j].FiscalYear {
			return results[i].FiscalYear < results[j].FiscalYear
		}
		return results[i].FiscalPeriod < results[j].FiscalPeriod
	})

	return results, nil
}

// ExtractStatements builds model.FinancialStatement list from Yahoo financial statements.
func (s *YahooService) ExtractStatements(ticker string, res *YahooQuoteSummaryResult) []model.FinancialStatement {
	if res == nil {
		return nil
	}

	var results []model.FinancialStatement

	// Map balance sheets by unix timestamp for fast merging
	bsMap := make(map[int64]YahooBalanceSheet)
	for _, bs := range res.BalanceSheetHistory.BalanceSheetStatements {
		bsMap[int64(bs.EndDate.Raw)] = bs
	}

	// Map cashflow statements by unix timestamp
	cfMap := make(map[int64]YahooCashflowStatement)
	for _, cf := range res.CashflowStatementHistory.CashflowStatements {
		cfMap[int64(cf.EndDate.Raw)] = cf
	}

	// Process Annual Statements
	for _, is := range res.IncomeStatementHistory.IncomeStatementHistory {
		t := time.Unix(int64(is.EndDate.Raw), 0).UTC()
		fy := t.Year()

		st := model.FinancialStatement{
			Ticker:        ticker,
			PeriodType:    model.PeriodAnnual,
			FiscalYear:    fy,
			FiscalPeriod:  model.FiscalPeriodFY,
			PeriodEndDate: t.Format("2006-01-02"),
			Revenue:       is.TotalRevenue.Raw,
			CostOfRevenue: is.CostOfRevenue.Raw,
			GrossProfit:   is.GrossProfit.Raw,
			OperatingIncome: is.OperatingIncome.Raw,
			NetIncome:     is.NetIncome.Raw,
			UpdatedAt:     time.Now(),
		}

		if st.GrossProfit == 0 && st.Revenue > 0 && st.CostOfRevenue > 0 {
			st.GrossProfit = st.Revenue - st.CostOfRevenue
		}

		// Merge matching Balance Sheet
		if bs, ok := bsMap[int64(is.EndDate.Raw)]; ok {
			st.CashAndEquiv = bs.CashAndCashEquivalents.Raw
			if st.CashAndEquiv == 0 {
				st.CashAndEquiv = bs.Cash.Raw + bs.ShortTermInvestments.Raw
			}
			st.TotalDebt = bs.LongTermDebt.Raw + bs.ShortLongTermDebt.Raw
			st.PreferredStock = bs.PreferredStock.Raw
			st.TotalEquity = bs.TotalStockholderEquity.Raw
		}

		// Merge matching Cashflow
		if cf, ok := cfMap[int64(is.EndDate.Raw)]; ok {
			st.OperatingCashFlow = cf.TotalCashFromOperatingActivities.Raw
			st.CapEx = cf.CapitalExpenditures.Raw
			st.DepreciationAmortization = cf.Depreciation.Raw
		}

		results = append(results, st)
	}

	// Process Quarterly Statements
	cfqMap := make(map[int64]YahooCashflowStatement)
	for _, cf := range res.CashflowStatementHistoryQuarterly.CashflowStatements {
		cfqMap[int64(cf.EndDate.Raw)] = cf
	}

	for _, is := range res.IncomeStatementHistoryQuarterly.IncomeStatementHistory {
		t := time.Unix(int64(is.EndDate.Raw), 0).UTC()
		fy := t.Year()
		month := int(t.Month())
		fp := "Q4"
		if month <= 3 {
			fp = "Q1"
		} else if month <= 6 {
			fp = "Q2"
		} else if month <= 9 {
			fp = "Q3"
		}

		st := model.FinancialStatement{
			Ticker:        ticker,
			PeriodType:    model.PeriodQuarterly,
			FiscalYear:    fy,
			FiscalPeriod:  fp,
			PeriodEndDate: t.Format("2006-01-02"),
			Revenue:       is.TotalRevenue.Raw,
			CostOfRevenue: is.CostOfRevenue.Raw,
			GrossProfit:   is.GrossProfit.Raw,
			OperatingIncome: is.OperatingIncome.Raw,
			NetIncome:     is.NetIncome.Raw,
			UpdatedAt:     time.Now(),
		}

		if cf, ok := cfqMap[int64(is.EndDate.Raw)]; ok {
			st.OperatingCashFlow = cf.TotalCashFromOperatingActivities.Raw
			st.CapEx = cf.CapitalExpenditures.Raw
			st.DepreciationAmortization = cf.Depreciation.Raw
		}

		results = append(results, st)
	}

	return results
}

// ExtractConsensusEstimates parses forward consensus estimates from earningsTrend.
func (s *YahooService) ExtractConsensusEstimates(ticker string, res *YahooQuoteSummaryResult, latestFY int) []model.ConsensusEstimate {
	if res == nil || len(res.EarningsTrend.Trend) == 0 {
		return nil
	}

	var estimates []model.ConsensusEstimate

	for _, item := range res.EarningsTrend.Trend {
		// Look for annual periods "0y", "+1y", "+2y", etc.
		if strings.HasSuffix(item.Period, "y") {
			var yearOffset int
			periodStr := strings.TrimSuffix(item.Period, "y")
			if n, err := strconv.Atoi(periodStr); err == nil {
				yearOffset = n
			}

			targetFY := latestFY + 1 + yearOffset
			if item.EndDate != "" {
				if t, err := time.Parse("2006-01-02", item.EndDate); err == nil {
					targetFY = t.Year()
				}
			}

			if item.RevenueEstimate.Avg.Raw > 0 || item.EarningsEstimate.Avg.Raw != 0 {
				est := model.ConsensusEstimate{
					Ticker:       ticker,
					FiscalYear:   targetFY,
					EstRevenue:   item.RevenueEstimate.Avg.Raw,
					EstEPS:       item.EarningsEstimate.Avg.Raw,
					Source:       model.SourceConsensus,
					UpdatedAt:    time.Now(),
				}
				estimates = append(estimates, est)
			}
		}
	}

	sort.Slice(estimates, func(i, j int) bool {
		return estimates[i].FiscalYear < estimates[j].FiscalYear
	})

	return estimates
}

// HistoricalPricePoint represents a date-indexed price point.
type HistoricalPricePoint struct {
	Date  time.Time
	Close float64
}

// FetchHistoricalMonthlyPrices retrieves 5-year monthly historical closing prices from Yahoo Chart API.
func (s *YahooService) FetchHistoricalMonthlyPrices(ctx context.Context, ticker string) ([]HistoricalPricePoint, error) {
	chartURL := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=5y&interval=1mo", url.PathEscape(ticker))
	opts := &RequestOptions{
		Headers: map[string]string{
			"User-Agent": WebUserAgent,
			"Accept":     "*/*",
			"Referer":    "https://finance.yahoo.com/",
		},
		Timeout: 10 * time.Second,
		Retries: 2,
	}

	data, err := s.client.Get(ctx, chartURL, opts)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Currency string `json:"currency"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Chart.Result) == 0 {
		return nil, fmt.Errorf("failed to parse historical chart response: %w", err)
	}

	res := raw.Chart.Result[0]
	if len(res.Timestamp) == 0 || len(res.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("empty chart history")
	}

	isMinorUnit := false
	mCurr := strings.TrimSpace(res.Meta.Currency)
	if mCurr == "GBp" || mCurr == "GBX" || mCurr == "ILA" || mCurr == "ZAc" || strings.EqualFold(mCurr, "GBp") || strings.EqualFold(mCurr, "GBX") {
		isMinorUnit = true
	} else if strings.HasSuffix(strings.ToUpper(ticker), ".L") {
		isMinorUnit = true
	}

	closes := res.Indicators.Quote[0].Close
	var points []HistoricalPricePoint
	for i, ts := range res.Timestamp {
		if i < len(closes) && closes[i] != nil && *closes[i] > 0 {
			price := *closes[i]
			if isMinorUnit {
				price /= 100.0
			}
			t := time.Unix(ts, 0).UTC()
			points = append(points, HistoricalPricePoint{
				Date:  t,
				Close: price,
			})
		}
	}

	return points, nil
}

// FindClosestPrice finds the closest historical closing price for a given target date or target FY.
func FindClosestPrice(points []HistoricalPricePoint, targetDate string, targetFY int) float64 {
	if len(points) == 0 {
		return 0
	}

	var targetTime time.Time
	if targetDate != "" {
		if t, err := time.Parse("2006-01-02", targetDate); err == nil {
			targetTime = t
		}
	}
	if targetTime.IsZero() && targetFY > 0 {
		targetTime = time.Date(targetFY, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	var bestPrice float64
	bestDiff := time.Duration(1<<63 - 1)

	for _, pt := range points {
		diff := pt.Date.Sub(targetTime)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			bestPrice = pt.Close
		}
	}

	return bestPrice
}

func httpNewGetRequest(ctx context.Context, url string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}
