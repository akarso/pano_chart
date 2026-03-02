package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"pano_chart/backend/domain"
)

// financeFlowItem is the raw JSON shape returned by the FinanceFlow API.
type financeFlowItem struct {
	Country        string `json:"country"`
	ReportName     string `json:"report_name"`
	Actual         string `json:"actual"`
	Previous       string `json:"previous"`
	Consensus      string `json:"consensus"`
	EconomicImpact string `json:"economicImpact"`
	ReportDate     string `json:"report_date"`
	Datetime       string `json:"datetime"`
}

// financeFlowResponse is the top-level API response.
type financeFlowResponse struct {
	Success bool              `json:"success"`
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    []financeFlowItem `json:"data"`
}

// FinanceFlowClient implements ports.EventProviderPort by calling the
// FinanceFlow financial calendar API.
type FinanceFlowClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

const defaultFinanceFlowBaseURL = "https://financeflowapi.com"

// NewFinanceFlowClient creates a client. baseURL may be empty to use the
// production endpoint.
func NewFinanceFlowClient(apiKey string, baseURL string, httpClient *http.Client) *FinanceFlowClient {
	if baseURL == "" {
		baseURL = defaultFinanceFlowBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &FinanceFlowClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  httpClient,
	}
}

// FetchEvents retrieves events for the given date range and optional country.
func (c *FinanceFlowClient) FetchEvents(ctx context.Context, dateFrom, dateTo time.Time, country string) ([]domain.Event, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/financial-calendar")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	q.Set("api_key", c.apiKey)
	q.Set("date_from", dateFrom.Format("2006-01-02"))
	q.Set("date_to", dateTo.Format("2006-01-02"))
	if country != "" {
		q.Set("country", country)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("financeflow request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	log.Printf("[FinanceFlow] GET %s → %d (%s)", u.Path, resp.StatusCode, latency)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("financeflow returned %d: %s", resp.StatusCode, string(body))
	}

	var ffResp financeFlowResponse
	if err := json.NewDecoder(resp.Body).Decode(&ffResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !ffResp.Success {
		return nil, fmt.Errorf("financeflow error: code=%d message=%s", ffResp.Code, ffResp.Message)
	}

	events := make([]domain.Event, 0, len(ffResp.Data))
	for _, item := range ffResp.Data {
		ev, err := mapFinanceFlowItem(item)
		if err != nil {
			log.Printf("[FinanceFlow] skipping event: %v", err)
			continue
		}
		events = append(events, ev)
	}

	log.Printf("[FinanceFlow] fetched %d events (%d raw)", len(events), len(ffResp.Data))
	return events, nil
}

const financeFlowTimeLayout = "2006-01-02 15:04:05"

// mapFinanceFlowItem converts a raw FinanceFlow item to a domain Event.
func mapFinanceFlowItem(item financeFlowItem) (domain.Event, error) {
	ts, err := time.Parse(financeFlowTimeLayout, item.Datetime)
	if err != nil {
		return domain.Event{}, fmt.Errorf("parse datetime %q: %w", item.Datetime, err)
	}

	impact := domain.ParseEventImpact(item.EconomicImpact)

	return domain.NewEvent("", item.Country, item.ReportName, impact, ts)
}
