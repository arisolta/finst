package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type FXService struct {
	client     *Client
	spotCache  map[string]float64
	avgCache   map[string]float64
	cacheLock  sync.RWMutex
}

func NewFXService(client *Client) *FXService {
	return &FXService{
		client:    client,
		spotCache: make(map[string]float64),
		avgCache:  make(map[string]float64),
	}
}

type FrankfurterLatestResponse struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

type FrankfurterHistoricalResponse struct {
	Amount    float64                       `json:"amount"`
	Base      string                        `json:"base"`
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Rates     map[string]map[string]float64 `json:"rates"`
}

// GetSpotRate returns the spot exchange rate from fromCurr to toCurr.
func (s *FXService) GetSpotRate(ctx context.Context, fromCurr, toCurr string) (float64, error) {
	from := strings.ToUpper(strings.TrimSpace(fromCurr))
	to := strings.ToUpper(strings.TrimSpace(toCurr))

	if from == "" || to == "" || from == to {
		return 1.0, nil
	}

	key := fmt.Sprintf("%s_%s", from, to)
	s.cacheLock.RLock()
	if rate, ok := s.spotCache[key]; ok {
		s.cacheLock.RUnlock()
		return rate, nil
	}
	s.cacheLock.RUnlock()

	url := fmt.Sprintf("https://api.frankfurter.app/latest?from=%s&to=%s", from, to)
	opts := &RequestOptions{
		Timeout: 10 * time.Second,
		Retries: 2,
	}

	data, err := s.client.Get(ctx, url, opts)
	if err != nil {
		// Fallback to dev subdomain
		url = fmt.Sprintf("https://api.frankfurter.dev/v1/latest?base=%s&symbols=%s", from, to)
		data, err = s.client.Get(ctx, url, opts)
	}

	var resp FrankfurterLatestResponse
	var frankfurterSuccess bool
	if err == nil {
		if jsonErr := json.Unmarshal(data, &resp); jsonErr == nil {
			if rate, ok := resp.Rates[to]; ok && rate > 0 {
				s.cacheLock.Lock()
				s.spotCache[key] = rate
				s.cacheLock.Unlock()
				return rate, nil
			}
		}
	}
	_ = frankfurterSuccess

	// Fallback to Yahoo Finance FX for exotic/non-ECB pairs (e.g. KZT, TWD, ARS, SAR)
	yRate, yErr := s.fetchYahooSpotRate(ctx, from, to)
	if yErr == nil && yRate > 0 {
		s.cacheLock.Lock()
		s.spotCache[key] = yRate
		s.cacheLock.Unlock()
		return yRate, nil
	}

	if err != nil {
		return 1.0, fmt.Errorf("failed to fetch spot FX rate %s -> %s: %w", from, to, err)
	}
	return 1.0, fmt.Errorf("exchange rate %s -> %s not available in response", from, to)
}

func (s *FXService) fetchYahooSpotRate(ctx context.Context, from, to string) (float64, error) {
	opts := &RequestOptions{
		Headers: map[string]string{
			"User-Agent": WebUserAgent,
			"Accept":     "*/*",
			"Referer":    "https://finance.yahoo.com/",
		},
		Timeout: 10 * time.Second,
		Retries: 2,
	}

	// 1. Direct pair e.g. USDKZT=X
	pair := fmt.Sprintf("%s%s=X", from, to)
	chartURL := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=1d&interval=1d", url.PathEscape(pair))
	if data, err := s.client.Get(ctx, chartURL, opts); err == nil {
		var raw struct {
			Chart struct {
				Result []struct {
					Meta struct {
						RegularMarketPrice float64 `json:"regularMarketPrice"`
					} `json:"meta"`
				} `json:"result"`
			} `json:"chart"`
		}
		if json.Unmarshal(data, &raw) == nil && len(raw.Chart.Result) > 0 && raw.Chart.Result[0].Meta.RegularMarketPrice > 0 {
			return raw.Chart.Result[0].Meta.RegularMarketPrice, nil
		}
	}

	// 2. Inverse pair e.g. KZTUSD=X
	invPair := fmt.Sprintf("%s%s=X", to, from)
	invURL := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=1d&interval=1d", url.PathEscape(invPair))
	if data, err := s.client.Get(ctx, invURL, opts); err == nil {
		var raw struct {
			Chart struct {
				Result []struct {
					Meta struct {
						RegularMarketPrice float64 `json:"regularMarketPrice"`
					} `json:"meta"`
				} `json:"result"`
			} `json:"chart"`
		}
		if json.Unmarshal(data, &raw) == nil && len(raw.Chart.Result) > 0 && raw.Chart.Result[0].Meta.RegularMarketPrice > 0 {
			return 1.0 / raw.Chart.Result[0].Meta.RegularMarketPrice, nil
		}
	}

	// 3. Cross rate via USD
	if from != "USD" && to != "USD" {
		rateFromUSD, err1 := s.fetchYahooSpotRate(ctx, "USD", from)
		rateToUSD, err2 := s.fetchYahooSpotRate(ctx, "USD", to)
		if err1 == nil && err2 == nil && rateFromUSD > 0 && rateToUSD > 0 {
			return rateToUSD / rateFromUSD, nil
		}
	}

	return 0, fmt.Errorf("yahoo fx rate not available for %s -> %s", from, to)
}

// GetAverageRate returns the historical average exchange rate for a given fiscal year.
func (s *FXService) GetAverageRate(ctx context.Context, fromCurr, toCurr string, fiscalYear int) (float64, error) {
	from := strings.ToUpper(strings.TrimSpace(fromCurr))
	to := strings.ToUpper(strings.TrimSpace(toCurr))

	if from == "" || to == "" || from == to {
		return 1.0, nil
	}

	key := fmt.Sprintf("%s_%s_%d", from, to, fiscalYear)
	s.cacheLock.RLock()
	if rate, ok := s.avgCache[key]; ok {
		s.cacheLock.RUnlock()
		return rate, nil
	}
	s.cacheLock.RUnlock()

	startDate := fmt.Sprintf("%d-01-01", fiscalYear)
	endDate := fmt.Sprintf("%d-12-31", fiscalYear)
	nowYear := time.Now().Year()
	if fiscalYear >= nowYear {
		// For current or future years, spot rate is the best available proxy
		return s.GetSpotRate(ctx, from, to)
	}

	url := fmt.Sprintf("https://api.frankfurter.app/%s..%s?from=%s&to=%s", startDate, endDate, from, to)
	opts := &RequestOptions{
		Timeout: 10 * time.Second,
		Retries: 2,
	}

	data, err := s.client.Get(ctx, url, opts)
	if err != nil {
		// Fallback to spot rate if historical range fails
		return s.GetSpotRate(ctx, from, to)
	}

	var resp FrankfurterHistoricalResponse
	if err := json.Unmarshal(data, &resp); err != nil || len(resp.Rates) == 0 {
		return s.GetSpotRate(ctx, from, to)
	}

	var sum float64
	var count int
	for _, dailyRates := range resp.Rates {
		if r, ok := dailyRates[to]; ok && r > 0 {
			sum += r
			count++
		}
	}

	if count == 0 {
		return s.GetSpotRate(ctx, from, to)
	}

	avgRate := sum / float64(count)

	s.cacheLock.Lock()
	s.avgCache[key] = avgRate
	s.cacheLock.Unlock()

	return avgRate, nil
}
