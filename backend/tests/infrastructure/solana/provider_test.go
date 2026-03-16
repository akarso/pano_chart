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
