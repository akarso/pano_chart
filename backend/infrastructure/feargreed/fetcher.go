package feargreed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"pano_chart/backend/application/usecases"
)

const defaultURL = "https://api.alternative.me/fng/?limit=1"

// apiResponse mirrors the upstream JSON envelope.
type apiResponse struct {
	Data []apiDataItem `json:"data"`
}

type apiDataItem struct {
	Value               string `json:"value"`
	ValueClassification string `json:"value_classification"`
	Timestamp           string `json:"timestamp"`
}

// Fetcher calls the alternative.me API and returns the current index.
type Fetcher struct {
	client *http.Client
	url    string
}

// NewFetcher creates a Fetcher. If client is nil, http.DefaultClient is used.
func NewFetcher(client *http.Client, url string) *Fetcher {
	if client == nil {
		client = http.DefaultClient
	}
	if url == "" {
		url = defaultURL
	}
	return &Fetcher{client: client, url: url}
}

// Execute implements FearGreedUseCase.
func (f *Fetcher) Execute(ctx context.Context) (*usecases.FearGreedResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("feargreed: new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feargreed: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feargreed: unexpected status %d", resp.StatusCode)
	}

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("feargreed: decode: %w", err)
	}
	if len(body.Data) == 0 {
		return nil, fmt.Errorf("feargreed: empty data")
	}

	item := body.Data[0]
	val, err := strconv.Atoi(item.Value)
	if err != nil {
		return nil, fmt.Errorf("feargreed: parse value %q: %w", item.Value, err)
	}
	ts, err := strconv.ParseInt(item.Timestamp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("feargreed: parse timestamp %q: %w", item.Timestamp, err)
	}

	return &usecases.FearGreedResult{
		Value:               val,
		ValueClassification: item.ValueClassification,
		Timestamp:           ts,
	}, nil
}
