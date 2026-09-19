package usecases

import (
	"context"
	"fmt"
	"time"

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

// fakePurchaseRepository implements ports.PurchaseRepository and
// ports.VerifiedPurchaseApplier for tests.
type fakePurchaseRepository struct {
	saved    []domain.Purchase
	lastID   int64
	existing map[string]domain.Purchase // key: "provider|txID"
	saveErr  error
	findErr  error
	applyErr error
	subs     *fakeSubscriptionRepository // required for ApplyVerifiedPurchase
}

func newFakePurchaseRepository() *fakePurchaseRepository {
	return &fakePurchaseRepository{
		existing: make(map[string]domain.Purchase),
	}
}

// newLinkedPaymentFakes wires purchase ApplyVerifiedPurchase to the
// subscription map so ActivateSubscription tests exercise ownership transfer.
func newLinkedPaymentFakes() (*fakePurchaseRepository, *fakeSubscriptionRepository) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	purchases.subs = subs
	return purchases, subs
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

func (f *fakePurchaseRepository) ApplyVerifiedPurchase(
	_ context.Context,
	previousOwnerID string,
	result domain.PaymentVerificationResult,
) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	if f.subs == nil {
		return fmt.Errorf("fake purchase repo missing linked subscription repo")
	}
	key := result.Provider() + "|" + result.ExternalTransactionID()
	existing, ok := f.existing[key]
	if !ok {
		return fmt.Errorf("purchase not found")
	}

	updated := domain.NewPurchaseUnsafe(
		existing.ID(),
		result.UserID(),
		existing.Provider(),
		existing.ExternalTransactionID(),
		existing.ProductID(),
		existing.PurchaseTime(),
		result.ExpirationTime(),
		existing.CreatedAt(),
		true,
	)
	f.existing[key] = updated
	for i, p := range f.saved {
		if p.Provider() == result.Provider() && p.ExternalTransactionID() == result.ExternalTransactionID() {
			f.saved[i] = updated
		}
	}

	sub, err := domain.NewSubscription(
		result.UserID(),
		result.Provider(),
		result.ProductID(),
		result.PurchaseTime(),
		result.ExpirationTime(),
	)
	if err != nil {
		return err
	}
	if err := f.subs.Upsert(context.Background(), sub); err != nil {
		return err
	}

	if previousOwnerID != "" && previousOwnerID != result.UserID() {
		now := time.Now().UTC()
		expired, err := domain.NewSubscription(
			previousOwnerID, result.Provider(), result.ProductID(), now, now,
		)
		if err != nil {
			return err
		}
		if err := f.subs.Upsert(context.Background(), expired); err != nil {
			return err
		}
	}
	return nil
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
