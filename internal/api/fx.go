package api

import (
	"context"
	"encoding/json"
	"fmt"
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

	if err != nil {
		return 1.0, fmt.Errorf("failed to fetch spot FX rate %s -> %s: %w", from, to, err)
	}

	var resp FrankfurterLatestResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 1.0, fmt.Errorf("failed to parse spot FX json: %w", err)
	}

	rate, ok := resp.Rates[to]
	if !ok || rate <= 0 {
		return 1.0, fmt.Errorf("exchange rate %s -> %s not available in response", from, to)
	}

	s.cacheLock.Lock()
	s.spotCache[key] = rate
	s.cacheLock.Unlock()

	return rate, nil
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
