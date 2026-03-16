package usecases

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/application/usecases"
)

func TestPaymentProviderRegistry_RegisterAndGet(t *testing.T) {
	reg := usecases.NewPaymentProviderRegistry()
	provider := &fakePaymentProvider{name: "test_provider"}

	reg.Register(provider)
	got, err := reg.Get("test_provider")

	require.NoError(t, err)
	assert.Equal(t, provider, got)
}

func TestPaymentProviderRegistry_GetUnknown(t *testing.T) {
	reg := usecases.NewPaymentProviderRegistry()

	_, err := reg.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestPaymentProviderRegistry_DuplicatePanics(t *testing.T) {
	reg := usecases.NewPaymentProviderRegistry()
	provider := &fakePaymentProvider{name: "dup"}
	reg.Register(provider)

	assert.Panics(t, func() {
		reg.Register(provider)
	})
}

func TestPaymentProviderRegistry_Names(t *testing.T) {
	reg := usecases.NewPaymentProviderRegistry()
	reg.Register(&fakePaymentProvider{name: "alpha"})
	reg.Register(&fakePaymentProvider{name: "beta"})

	names := reg.Names()
	assert.Len(t, names, 2)
	assert.ElementsMatch(t, []string{"alpha", "beta"}, names)
}

func TestPaymentProviderRegistry_EmptyNames(t *testing.T) {
	reg := usecases.NewPaymentProviderRegistry()
	assert.Empty(t, reg.Names())
}
