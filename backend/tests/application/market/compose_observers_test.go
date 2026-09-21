package market_test

import (
	"errors"
	"testing"

	appmarket "pano_chart/backend/application/market"
	mkt "pano_chart/backend/domain/market"
)

type countingObserver struct {
	n int
}

func (c *countingObserver) Update(string, mkt.Regime, string, int64) error {
	c.n++
	return nil
}

func TestComposeObservers_FanOut(t *testing.T) {
	a, b := &countingObserver{}, &countingObserver{}
	o := appmarket.ComposeObservers(a, b, nil)
	if err := o.Update("1h", mkt.RegimeTrend, "up", 1); err != nil {
		t.Fatal(err)
	}
	if a.n != 1 || b.n != 1 {
		t.Fatalf("a=%d b=%d", a.n, b.n)
	}
}

func TestComposeObservers_NilAndSingle(t *testing.T) {
	if appmarket.ComposeObservers() != nil {
		t.Fatal("empty should be nil")
	}
	a := &countingObserver{}
	o := appmarket.ComposeObservers(a)
	_ = o.Update("1h", mkt.RegimeSideways, "neutral", 1)
	if a.n != 1 {
		t.Fatal(a.n)
	}
}

type errObserver struct{ err error }

func (e *errObserver) Update(string, mkt.Regime, string, int64) error { return e.err }

func TestComposeObservers_FirstErrorAfterAll(t *testing.T) {
	a, b := &countingObserver{}, &countingObserver{}
	fail := &errObserver{err: errors.New("boom")}
	o := appmarket.ComposeObservers(fail, a, b)
	err := o.Update("1h", mkt.RegimeTrend, "up", 1)
	if err == nil {
		t.Fatal("expected first error")
	}
	if a.n != 1 || b.n != 1 {
		t.Fatalf("all observers should still run: a=%d b=%d", a.n, b.n)
	}
}
