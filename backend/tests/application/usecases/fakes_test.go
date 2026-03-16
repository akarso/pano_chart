package usecases

import (
	"context"

	"pano_chart/backend/domain"
)

// fakePaymentProvider implements ports.PaymentProviderPort for tests.
type fakePaymentProvider struct {
	name   string
	result domain.PaymentVerificationResult
	err    error
}

func (f *fakePaymentProvider) VerifyPurchase(_ context.Context, _ string, _ string) (domain.PaymentVerificationResult, error) {
	if f.err != nil {
		return domain.PaymentVerificationResult{}, f.err
	}
	return f.result, nil
}

func (f *fakePaymentProvider) ProviderName() string { return f.name }

// fakePurchaseRepository implements ports.PurchaseRepository for tests.
type fakePurchaseRepository struct {
	saved    []domain.Purchase
	lastID   int64
	existing map[string]domain.Purchase // key: "provider|txID"
	saveErr  error
	findErr  error
}

func newFakePurchaseRepository() *fakePurchaseRepository {
	return &fakePurchaseRepository{
		existing: make(map[string]domain.Purchase),
	}
}

func (f *fakePurchaseRepository) Save(_ context.Context, p domain.Purchase) (int64, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	f.lastID++
	f.saved = append(f.saved, p)
	key := p.Provider() + "|" + p.ExternalTransactionID()
	f.existing[key] = p
	return f.lastID, nil
}

func (f *fakePurchaseRepository) FindByTransactionID(
	_ context.Context,
	provider, externalTransactionID string,
) (domain.Purchase, bool, error) {
	if f.findErr != nil {
		return domain.Purchase{}, false, f.findErr
	}
	key := provider + "|" + externalTransactionID
	p, ok := f.existing[key]
	return p, ok, nil
}

// fakeSubscriptionRepository implements ports.SubscriptionRepository for tests.
type fakeSubscriptionRepository struct {
	subs      map[string]domain.Subscription // key: userID
	upsertErr error
	findErr   error
}

func newFakeSubscriptionRepository() *fakeSubscriptionRepository {
	return &fakeSubscriptionRepository{
		subs: make(map[string]domain.Subscription),
	}
}

func (f *fakeSubscriptionRepository) Upsert(_ context.Context, sub domain.Subscription) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.subs[sub.UserID()] = sub
	return nil
}

func (f *fakeSubscriptionRepository) FindByUserID(
	_ context.Context,
	userID string,
) (domain.Subscription, bool, error) {
	if f.findErr != nil {
		return domain.Subscription{}, false, f.findErr
	}
	sub, ok := f.subs[userID]
	return sub, ok, nil
}
