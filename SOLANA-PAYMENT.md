# Solana Payment Implementation

> **Status**: Temporarily removed to avoid Google Play policy risks.
> This document preserves the full implementation for future restoration.

---

## Overview

The Solana payment system allowed users to pay for a Pano Pro monthly subscription
($4.99/month) using SOL on the Solana blockchain. It integrated as one of the
`PaymentProviderPort` implementations alongside Google Play billing.

### Flow

1. **UpgradeScreen** shows a "Pay with Solana" button below the Google Play subscribe button.
2. Tapping navigates to **SolanaPaymentScreen** which fetches the current SOL price from the backend.
3. User sees: the USD price, required SOL amount, and the merchant wallet address (tap-to-copy).
4. User sends SOL from their own wallet, then pastes the Solana TX signature into a text field.
5. Frontend calls the generic `/api/payments/verify` endpoint with `provider: "solana"`.
6. Backend's **Solana Provider** verifies the TX on-chain via Solana RPC (`getTransaction`).
7. If valid, the subscription is activated for 30 days from the TX block time.

### Environment Variables

| Variable                  | Description                          | Default                                    |
|---------------------------|--------------------------------------|--------------------------------------------|
| `SOLANA_WALLET_ADDRESS`   | Merchant wallet receiving payments   | *(required — provider disabled if unset)*  |
| `SOLANA_RPC_URL`          | Solana JSON-RPC endpoint             | `https://api.mainnet-beta.solana.com`      |

### Configuration (hardcoded in main.go)

| Setting            | Value  |
|--------------------|--------|
| PriceUSD           | 4.99   |
| SubscriptionDays   | 30     |
| Tolerance          | 5%     |
| Product ID         | `pano_pro_sol` |

---

## Backend Files

### `backend/infrastructure/solana/provider.go`

Full Solana payment provider implementing `PaymentProviderPort`.

```go
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
```

### `backend/tests/infrastructure/solana/provider_test.go`

```go
package solana_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/infrastructure/solana"
)

func TestProviderName(t *testing.T) {
	p := solana.NewProvider(solana.Config{}, nil)
	assert.Equal(t, "solana", p.ProviderName())
}

func TestVerifyPurchase_EmptyToken(t *testing.T) {
	p := solana.NewProvider(solana.Config{WalletAddress: "abc"}, nil)
	result, err := p.VerifyPurchase(context.Background(), "", "user1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestVerifyPurchase_TxNotFound(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
	}))
	defer rpc.Close()

	p := solana.NewProvider(solana.Config{
		WalletAddress: "MyWallet123",
		RPCURL:        rpc.URL,
	}, nil)

	result, err := p.VerifyPurchase(context.Background(), "badSig", "user1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestVerifyPurchase_TxWithError(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"jsonrpc":"2.0","id":1,"result":{"blockTime":1700000000,"meta":{"err":{"InstructionError":[0,"Custom"]},"preBalances":[100],"postBalances":[100],"postTokenBalances":[]},"transaction":{"message":{"accountKeys":["MyWallet123"]}}}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer rpc.Close()

	p := solana.NewProvider(solana.Config{
		WalletAddress: "MyWallet123",
		RPCURL:        rpc.URL,
	}, nil)

	result, err := p.VerifyPurchase(context.Background(), "errSig", "user1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestVerifyPurchase_InsufficientAmount(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"jsonrpc":"2.0","id":1,"result":{"blockTime":1700000000,"meta":{"err":null,"preBalances":[0,500000000],"postBalances":[10000000,490000000],"postTokenBalances":[]},"transaction":{"message":{"accountKeys":["MyWallet123","Sender"]}}}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer rpc.Close()

	priceAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"solana":{"usd":100}}`))
	}))
	defer priceAPI.Close()

	p := solana.NewProvider(solana.Config{
		WalletAddress: "MyWallet123",
		PriceUSD:      4.99,
		RPCURL:        rpc.URL,
		PriceURL:      priceAPI.URL,
	}, nil)

	result, err := p.VerifyPurchase(context.Background(), "lowSig", "user1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestVerifyPurchase_ValidPayment(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpcReq map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&rpcReq))
		assert.Equal(t, "getTransaction", rpcReq["method"])

		w.Header().Set("Content-Type", "application/json")
		resp := `{"jsonrpc":"2.0","id":1,"result":{"blockTime":1700000000,"meta":{"err":null,"preBalances":[0,1000000000],"postBalances":[50000000,950000000],"postTokenBalances":[]},"transaction":{"message":{"accountKeys":["MyWallet123","SenderWallet"]}}}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer rpc.Close()

	priceAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"solana":{"usd":130}}`))
	}))
	defer priceAPI.Close()

	p := solana.NewProvider(solana.Config{
		WalletAddress:    "MyWallet123",
		PriceUSD:         4.99,
		SubscriptionDays: 30,
		RPCURL:           rpc.URL,
		PriceURL:         priceAPI.URL,
	}, nil)

	result, err := p.VerifyPurchase(context.Background(), "goodSig123abc", "user42")
	require.NoError(t, err)
	assert.True(t, result.Valid())
	assert.Equal(t, "solana", result.Provider())
	assert.Equal(t, "goodSig123abc", result.ExternalTransactionID())
	assert.Equal(t, "pano_pro_sol", result.ProductID())
	assert.Equal(t, "user42", result.UserID())
	assert.False(t, result.PurchaseTime().IsZero())
	assert.False(t, result.ExpirationTime().IsZero())
	assert.Equal(t, 30, int(result.ExpirationTime().Sub(result.PurchaseTime()).Hours()/24))
}

func TestVerifyPurchase_WalletNotInTx(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"jsonrpc":"2.0","id":1,"result":{"blockTime":1700000000,"meta":{"err":null,"preBalances":[100,200],"postBalances":[50,250],"postTokenBalances":[]},"transaction":{"message":{"accountKeys":["OtherWallet","AnotherWallet"]}}}}`
		_, _ = w.Write([]byte(resp))
	}))
	defer rpc.Close()

	p := solana.NewProvider(solana.Config{
		WalletAddress: "MyWallet123",
		RPCURL:        rpc.URL,
	}, nil)

	result, err := p.VerifyPurchase(context.Background(), "noWalletSig", "user1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestRequiredSOL(t *testing.T) {
	p := solana.NewProvider(solana.Config{PriceUSD: 4.99}, nil)
	sol := p.RequiredSOL(130.0)
	assert.Greater(t, sol, 0.0)
	assert.InDelta(t, 4.99/130.0, sol, 0.000001)
	assert.Equal(t, 0.0, p.RequiredSOL(0))
}

func TestFormatLamports(t *testing.T) {
	assert.Equal(t, int64(1_000_000_000), solana.FormatLamports(1.0))
	assert.Equal(t, int64(500_000_000), solana.FormatLamports(0.5))
}

func TestFormatSOL(t *testing.T) {
	assert.Equal(t, "1.000000", solana.FormatSOL(1_000_000_000))
	assert.Equal(t, "0.038385", solana.FormatSOL(38_385_000))
}
```

### `backend/adapters/http/sol_price_handler.go`

```go
package http

import (
	"encoding/json"
	"net/http"

	"pano_chart/backend/infrastructure/solana"
)

// SolPriceResponse is the JSON body returned by the SOL price endpoint.
type SolPriceResponse struct {
	SolPrice    float64 `json:"sol_price"`
	RequiredSOL float64 `json:"required_sol"`
	PriceUSD    float64 `json:"price_usd"`
	Wallet      string  `json:"wallet"`
}

// NewSolPriceHandler returns a handler that responds with the current
// SOL price and the required SOL amount for a subscription.
func NewSolPriceHandler(provider *solana.Provider, wallet string, priceUSD float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		solPrice, err := provider.GetSOLPricePublic(r.Context())
		if err != nil {
			http.Error(w, "failed to fetch SOL price", http.StatusServiceUnavailable)
			return
		}

		resp := SolPriceResponse{
			SolPrice:    solPrice,
			RequiredSOL: provider.RequiredSOL(solPrice),
			PriceUSD:    priceUSD,
			Wallet:      wallet,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}
}
```

### `backend/cmd/api/main.go` — Solana wiring (removed section)

```go
// --- Solana payment provider ---
solWallet := os.Getenv("SOLANA_WALLET_ADDRESS")
solRPC := os.Getenv("SOLANA_RPC_URL")
var solProvider *solana.Provider
if solWallet != "" {
    solCfg := solana.Config{
        WalletAddress:    solWallet,
        PriceUSD:         4.99,
        SubscriptionDays: 30,
        RPCURL:           solRPC,
    }
    solProvider = solana.NewProvider(solCfg, nil)
    providerRegistry.Register(solProvider)
    log.Printf("[main] Solana provider registered (wallet=%s)\n", solWallet)
} else {
    log.Println("[main] Solana provider not configured (SOLANA_WALLET_ADDRESS not set)")
}

// ... and in handlers section:
if solProvider != nil {
    mux.Handle("/api/sol/price", adhttp.NewSolPriceHandler(solProvider, solWallet, 4.99))
    log.Println("[main] /api/sol/price endpoint registered")
}
```

---

## Frontend Files

### `frontend/lib/features/billing/solana_payment_screen.dart`

```dart
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'api/sol_price_info.dart';
import 'billing_manager.dart';

/// Screen guiding the user through a Solana payment:
///
/// 1. Fetches the current SOL price and required amount from the backend.
/// 2. Displays the merchant wallet address (tap to copy).
/// 3. Provides a text field for the user to paste their TX signature.
/// 4. Verifies the payment server-side and activates the subscription.
class SolanaPaymentScreen extends StatefulWidget {
  final BillingManager billingManager;

  const SolanaPaymentScreen({Key? key, required this.billingManager})
      : super(key: key);

  @override
  State<SolanaPaymentScreen> createState() => _SolanaPaymentScreenState();
}

class _SolanaPaymentScreenState extends State<SolanaPaymentScreen> {
  final _txController = TextEditingController();
  SolPriceInfo? _priceInfo;
  bool _loading = true;
  bool _verifying = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _fetchPrice();
  }

  @override
  void dispose() {
    _txController.dispose();
    super.dispose();
  }

  Future<void> _fetchPrice() async {
    try {
      final info = await widget.billingManager.getSolPrice();
      if (mounted) {
        setState(() {
          _priceInfo = info;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Could not fetch SOL price. Please try again later.';
          _loading = false;
        });
      }
    }
  }

  Future<void> _verify() async {
    final sig = _txController.text.trim();
    if (sig.isEmpty) {
      setState(() => _error = 'Please enter your transaction signature.');
      return;
    }

    setState(() {
      _verifying = true;
      _error = null;
    });

    final success = await widget.billingManager.verifySolanaPayment(sig);

    if (!mounted) return;

    if (success) {
      Navigator.of(context).pop(true);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Payment verified! Subscription active.')),
      );
    } else {
      setState(() {
        _verifying = false;
        _error =
            'Verification failed. Make sure the TX is confirmed and the '
            'correct amount was sent to the wallet shown above.';
      });
    }
  }

  void _copyWallet() {
    if (_priceInfo == null) return;
    Clipboard.setData(ClipboardData(text: _priceInfo!.wallet));
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Wallet address copied to clipboard')),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: const Text('Pay with Solana'),
        backgroundColor: Colors.black,
        foregroundColor: Colors.white,
        elevation: 0,
      ),
      body: SafeArea(
        child: _loading
            ? const Center(child: CircularProgressIndicator())
            : SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: _buildContent(),
              ),
      ),
    );
  }

  Widget _buildContent() {
    if (_priceInfo == null) {
      return Center(
        child: Text(
          _error ?? 'Unable to load pricing.',
          style: const TextStyle(color: Colors.redAccent, fontSize: 16),
          textAlign: TextAlign.center,
        ),
      );
    }

    final info = _priceInfo!;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SizedBox(height: 24),
        // Price info
        Center(
          child: Column(
            children: [
              const Icon(Icons.currency_exchange,
                  color: Color(0xFF00E6C0), size: 56),
              const SizedBox(height: 12),
              Text(
                '\$${info.priceUSD.toStringAsFixed(2)} / month',
                style: const TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                '≈ ${info.requiredSOL.toStringAsFixed(6)} SOL',
                style: const TextStyle(
                  fontSize: 18,
                  color: Color(0xFF00E6C0),
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                '1 SOL ≈ \$${info.solPrice.toStringAsFixed(2)}',
                style: const TextStyle(color: Colors.white38, fontSize: 13),
              ),
            ],
          ),
        ),
        const SizedBox(height: 32),

        // Instructions
        const Text(
          'How to pay',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
        const SizedBox(height: 8),
        _stepText('1. Send the exact SOL amount above to the wallet below.'),
        _stepText('2. Wait for the transaction to be confirmed.'),
        _stepText('3. Paste the transaction signature below and tap Verify.'),
        const SizedBox(height: 24),

        // Wallet address
        const Text(
          'Send SOL to:',
          style: TextStyle(color: Colors.white54, fontSize: 13),
        ),
        const SizedBox(height: 6),
        GestureDetector(
          onTap: _copyWallet,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.05),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: Colors.white12),
            ),
            child: Row(
              children: [
                Expanded(
                  child: SelectableText(
                    info.wallet,
                    style: const TextStyle(
                      color: Color(0xFF00E6C0),
                      fontSize: 13,
                      fontFamily: 'monospace',
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                const Icon(Icons.copy, color: Colors.white54, size: 18),
              ],
            ),
          ),
        ),
        const SizedBox(height: 24),

        // TX signature input
        const Text(
          'Transaction Signature:',
          style: TextStyle(color: Colors.white54, fontSize: 13),
        ),
        const SizedBox(height: 6),
        TextField(
          controller: _txController,
          style: const TextStyle(color: Colors.white, fontFamily: 'monospace', fontSize: 13),
          decoration: InputDecoration(
            hintText: 'Paste your Solana TX signature here...',
            hintStyle: const TextStyle(color: Colors.white24),
            filled: true,
            fillColor: Colors.white.withValues(alpha: 0.05),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Colors.white12),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Colors.white12),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: const BorderSide(color: Color(0xFF00E6C0)),
            ),
          ),
        ),
        const SizedBox(height: 24),

        // Verify button
        SizedBox(
          width: double.infinity,
          child: ElevatedButton(
            onPressed: _verifying ? null : _verify,
            style: ElevatedButton.styleFrom(
              backgroundColor: const Color(0xFF00E6C0),
              foregroundColor: Colors.black,
              padding: const EdgeInsets.symmetric(vertical: 16),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
              textStyle:
                  const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
            ),
            child: _verifying
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Colors.black),
                  )
                : const Text('Verify Payment'),
          ),
        ),

        // Error message
        if (_error != null) ...[
          const SizedBox(height: 16),
          Text(
            _error!,
            style: const TextStyle(color: Colors.redAccent, fontSize: 13),
            textAlign: TextAlign.center,
          ),
        ],
        const SizedBox(height: 32),
      ],
    );
  }

  Widget _stepText(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Text(
        text,
        style: const TextStyle(color: Colors.white54, fontSize: 13),
      ),
    );
  }
}
```

### `frontend/lib/features/billing/api/sol_price_info.dart`

```dart
/// Data returned by the `/api/sol/price` endpoint.
class SolPriceInfo {
  final double solPrice;
  final double requiredSOL;
  final double priceUSD;
  final String wallet;

  const SolPriceInfo({
    required this.solPrice,
    required this.requiredSOL,
    required this.priceUSD,
    required this.wallet,
  });

  factory SolPriceInfo.fromJson(Map<String, dynamic> json) {
    return SolPriceInfo(
      solPrice: (json['sol_price'] as num).toDouble(),
      requiredSOL: (json['required_sol'] as num).toDouble(),
      priceUSD: (json['price_usd'] as num).toDouble(),
      wallet: json['wallet'] as String,
    );
  }
}
```

### Integration points removed from shared files

**`billing_manager.dart`** — removed `getSolPrice()` and `verifySolanaPayment()` methods.

**`upgrade_screen.dart`** — removed "Pay with Solana" `OutlinedButton.icon` and `_onPayWithSolana()` method.

**`subscription_api.dart`** — removed `Future<SolPriceInfo> getSolPrice()` from abstract class.

**`http_subscription_api.dart`** — removed `getSolPrice()` HTTP implementation.

---

## Restoration Guide

To bring Solana payments back:

1. Recreate `backend/infrastructure/solana/` with `provider.go` (copy from above).
2. Recreate `backend/tests/infrastructure/solana/provider_test.go` (copy from above).
3. Recreate `backend/adapters/http/sol_price_handler.go` (copy from above).
4. In `backend/cmd/api/main.go`:
   - Add import `"pano_chart/backend/infrastructure/solana"`
   - Add the Solana provider init block after Google Play (see above).
   - Add `/api/sol/price` handler registration in the mux section.
5. Recreate `frontend/lib/features/billing/api/sol_price_info.dart`.
6. Recreate `frontend/lib/features/billing/solana_payment_screen.dart`.
7. In `frontend/lib/features/billing/api/subscription_api.dart`:
   - Add `import 'sol_price_info.dart';`
   - Add `Future<SolPriceInfo> getSolPrice();` to abstract class.
8. In `frontend/lib/features/billing/infrastructure/http_subscription_api.dart`:
   - Add `import '../api/sol_price_info.dart';`
   - Add `getSolPrice()` override method.
9. In `frontend/lib/features/billing/billing_manager.dart`:
   - Add `import 'api/sol_price_info.dart';`
   - Add `getSolPrice()` and `verifySolanaPayment()` methods.
10. In `frontend/lib/features/billing/upgrade_screen.dart`:
    - Add `import 'solana_payment_screen.dart';`
    - Add "Pay with Solana" button and `_onPayWithSolana()` method.
11. Update test fakes and test cases accordingly.
