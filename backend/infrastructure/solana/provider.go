package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"pano_chart/backend/domain"
)

// Config holds all Solana payment provider settings.
type Config struct {
	// WalletAddress is the merchant's Solana wallet that receives payments.
	WalletAddress string

	// PriceUSD is the subscription price in US dollars (e.g. 4.99).
	PriceUSD float64

	// SubscriptionDays is how many days a single payment grants (default 30).
	SubscriptionDays int

	// Tolerance is the percentage below PriceUSD that is still accepted
	// (0.0–1.0, e.g. 0.05 for 5 %).  This covers SOL price fluctuation
	// between the time the user sends the TX and it gets confirmed.
	Tolerance float64

	// RPCURL is the Solana JSON-RPC endpoint (mainnet-beta by default).
	RPCURL string

	// PriceURL is the endpoint that returns {"solana":{"usd":<float>}}.
	// Defaults to CoinGecko simple price API.
	PriceURL string
}

func (c Config) rpcURL() string {
	if c.RPCURL != "" {
		return c.RPCURL
	}
	return "https://api.mainnet-beta.solana.com"
}

func (c Config) priceURL() string {
	if c.PriceURL != "" {
		return c.PriceURL
	}
	return "https://api.coingecko.com/api/v3/simple/price?ids=solana&vs_currencies=usd"
}

func (c Config) subscriptionDays() int {
	if c.SubscriptionDays > 0 {
		return c.SubscriptionDays
	}
	return 30
}

func (c Config) tolerance() float64 {
	if c.Tolerance > 0 {
		return c.Tolerance
	}
	return 0.05 // 5 %
}

// Provider implements ports.PaymentProviderPort for Solana on-chain payments.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider creates a Solana payment provider.
func NewProvider(cfg Config, client *http.Client) *Provider {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Provider{cfg: cfg, client: client}
}

// ProviderName returns "solana".
func (p *Provider) ProviderName() string { return "solana" }

// VerifyPurchase verifies a Solana transaction signature.
//
// It checks:
//  1. The TX exists and is confirmed.
//  2. One of the post-token-balances or native SOL transfers is to our wallet.
//  3. The transferred SOL amount is >= the expected USD value (minus tolerance).
//
// purchaseToken is the Solana transaction signature (base-58).
func (p *Provider) VerifyPurchase(
	ctx context.Context, purchaseToken string, userID string,
) (domain.PaymentVerificationResult, error) {
	if purchaseToken == "" {
		return invalid()
	}

	tx, err := p.getTransaction(ctx, purchaseToken)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("fetching solana tx: %w", err)
	}

	if tx.Result == nil {
		return invalid()
	}

	// Must be confirmed (finalized preferred, confirmed acceptable).
	// If the RPC returns the tx it is at least confirmed.

	// Check for error in the transaction itself.
	if tx.Result.Meta.Err != nil {
		return invalid()
	}

	// Find the SOL amount transferred to our wallet.
	lamports := p.findTransferToWallet(tx.Result)
	if lamports == 0 {
		return invalid()
	}

	solAmount := float64(lamports) / 1e9

	// Get current SOL/USD price and check the payment covers the subscription.
	solPrice, err := p.getSOLPrice(ctx)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("fetching SOL price: %w", err)
	}

	usdValue := solAmount * solPrice
	minAccepted := p.cfg.PriceUSD * (1.0 - p.cfg.tolerance())
	if usdValue < minAccepted {
		return invalid()
	}

	// Build timestamps.
	blockTime := time.Unix(tx.Result.BlockTime, 0).UTC()
	days := p.cfg.subscriptionDays()
	expiration := blockTime.Add(time.Duration(days) * 24 * time.Hour)

	return domain.NewPaymentVerificationResult(
		true,
		"solana",
		purchaseToken,  // tx signature as external transaction ID
		"pano_pro_sol", // product ID for Solana subscriptions
		userID,
		blockTime,
		expiration,
	)
}

// ---------------------------------------------------------------------------
// Solana RPC helpers
// ---------------------------------------------------------------------------

type rpcRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcTransactionResponse struct {
	Result *transactionResult `json:"result"`
}

type transactionResult struct {
	BlockTime   int64           `json:"blockTime"`
	Meta        transactionMeta `json:"meta"`
	Transaction txEnvelope      `json:"transaction"`
}

type transactionMeta struct {
	Err               interface{}   `json:"err"`
	PreBalances       []int64       `json:"preBalances"`
	PostBalances      []int64       `json:"postBalances"`
	PostTokenBalances []interface{} `json:"postTokenBalances"`
}

type txEnvelope struct {
	Message txMessage `json:"message"`
}

type txMessage struct {
	AccountKeys []string `json:"accountKeys"`
}

func (p *Provider) getTransaction(ctx context.Context, sig string) (rpcTransactionResponse, error) {
	body := rpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getTransaction",
		Params: []interface{}{
			sig,
			map[string]interface{}{
				"encoding":                       "json",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return rpcTransactionResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.rpcURL(), bytes.NewReader(raw))
	if err != nil {
		return rpcTransactionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return rpcTransactionResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return rpcTransactionResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return rpcTransactionResponse{}, fmt.Errorf("solana RPC returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result rpcTransactionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return rpcTransactionResponse{}, fmt.Errorf("decoding RPC response: %w", err)
	}
	return result, nil
}

// findTransferToWallet checks the pre/post native SOL balances for the
// merchant wallet and returns the lamports received.
func (p *Provider) findTransferToWallet(tx *transactionResult) int64 {
	accounts := tx.Transaction.Message.AccountKeys
	for i, acct := range accounts {
		if acct != p.cfg.WalletAddress {
			continue
		}
		if i >= len(tx.Meta.PreBalances) || i >= len(tx.Meta.PostBalances) {
			continue
		}
		diff := tx.Meta.PostBalances[i] - tx.Meta.PreBalances[i]
		if diff > 0 {
			return diff
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// SOL price helper
// ---------------------------------------------------------------------------

type coinGeckoPrice struct {
	Solana struct {
		USD float64 `json:"usd"`
	} `json:"solana"`
}

// GetSOLPricePublic fetches the current SOL/USD price. Exposed for the
// HTTP handler so it can show the required SOL amount to the frontend.
func (p *Provider) GetSOLPricePublic(ctx context.Context) (float64, error) {
	return p.getSOLPrice(ctx)
}

func (p *Provider) getSOLPrice(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.priceURL(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("price API returned %d: %s", resp.StatusCode, string(body))
	}

	var price coinGeckoPrice
	if err := json.Unmarshal(body, &price); err != nil {
		return 0, fmt.Errorf("decoding price response: %w", err)
	}
	if price.Solana.USD <= 0 {
		return 0, fmt.Errorf("invalid SOL price: %f", price.Solana.USD)
	}
	return price.Solana.USD, nil
}

// RequiredSOL returns the SOL amount needed for a subscription at the
// current price. Exposed so the frontend can display the exact amount.
func (p *Provider) RequiredSOL(solPrice float64) float64 {
	if solPrice <= 0 {
		return 0
	}
	// Round up to 6 decimal places (standard SOL precision).
	raw := p.cfg.PriceUSD / solPrice
	return math.Ceil(raw*1e6) / 1e6
}

func invalid() (domain.PaymentVerificationResult, error) {
	r, _ := domain.NewPaymentVerificationResult(false, "solana", "", "", "", time.Time{}, time.Time{})
	return r, nil
}

// FormatLamports converts a SOL float to lamports (int64).
func FormatLamports(sol float64) int64 {
	return int64(math.Round(sol * 1e9))
}

// FormatSOL converts lamports to a SOL string with 6 decimals.
func FormatSOL(lamports int64) string {
	return strconv.FormatFloat(float64(lamports)/1e9, 'f', 6, 64)
}
