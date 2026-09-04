package notifications_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/notifications"
	"pano_chart/backend/domain"
)

// realisticEvalProvider builds an evaluation mix meant to resemble a real
// symbol universe rather than a hand-picked fixture: most symbols are not
// trending, a minority are — mirroring how a genuinely bullish market
// actually looks (never uniformly one regime).
type realisticEvalProvider struct{}

func (realisticEvalProvider) GetLatestEvaluations(_ string) ([]domain.EvaluationSnapshot, error) {
	rnd := rand.New(rand.NewSource(1))
	var evals []domain.EvaluationSnapshot
	add := func(n int, mk func() domain.EvaluationSnapshot) {
		for i := 0; i < n; i++ {
			evals = append(evals, mk())
		}
	}
	// 50 clearly trending symbols (bullish).
	add(50, func() domain.EvaluationSnapshot {
		return domain.EvaluationSnapshot{
			TrendScore: 0.6 + rnd.Float64()*0.3, SidewaysScore: rnd.Float64() * 0.2,
			CompressionScore: rnd.Float64() * 0.15, BreakoutUpScore: rnd.Float64() * 0.15,
			Bias: "up", RecentReturn: 1.0 + rnd.Float64(),
		}
	})
	// 20 expanding/breaking out.
	add(20, func() domain.EvaluationSnapshot {
		return domain.EvaluationSnapshot{
			BreakoutUpScore: 0.6 + rnd.Float64()*0.3, TrendScore: rnd.Float64() * 0.25,
			SidewaysScore: rnd.Float64() * 0.15, Bias: "up", RecentReturn: 0.5 + rnd.Float64(),
		}
	})
	// 15 compressing.
	add(15, func() domain.EvaluationSnapshot {
		return domain.EvaluationSnapshot{
			CompressionScore: 0.6 + rnd.Float64()*0.3, TrendScore: rnd.Float64() * 0.2,
			SidewaysScore: rnd.Float64() * 0.15, Bias: "neutral",
		}
	})
	// 15 flat/sideways.
	add(15, func() domain.EvaluationSnapshot {
		return domain.EvaluationSnapshot{
			SidewaysScore: 0.5 + rnd.Float64()*0.3, TrendScore: rnd.Float64() * 0.2,
			CompressionScore: rnd.Float64() * 0.2, Bias: "neutral",
		}
	})
	return evals, nil
}

// TestScheduler_RealisticStrongTrendMarket_FiresUptrend is the data-driven
// sanity check the PR-073 CR round asked for: it runs the real
// MarketStateService (not a hand-injected mkt.Summary fixture) over a
// plausible, mixed-regime evaluation set and drives it through the real
// Scheduler with default config, confirming that a genuinely bullish (but
// realistically diluted, 50/100 symbols trending) market actually clears
// the default thresholds end-to-end. Before PR-073's threshold recalibration
// (0.75 → 0.35) and its removal of the State != Indecisive gate for
// Uptrend/Downtrend, this exact scenario did NOT fire — see docs/v2/PR-073.md.
func TestScheduler_RealisticStrongTrendMarket_FiresUptrend(t *testing.T) {
	market := appmarket.NewMarketStateService(realisticEvalProvider{})

	summary, err := market.Calculate("1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Bias != "up" {
		t.Fatalf("expected bias up for a majority-bullish mix, got %q", summary.Bias)
	}
	if summary.Breadth.Trend < notifications.DefaultNotificationConfig("").UptrendMinDominance {
		t.Fatalf("expected Breadth.Trend (%.4f) to clear the default UptrendMinDominance (%.4f) for a realistic strong-trend mix",
			summary.Breadth.Trend, notifications.DefaultNotificationConfig("").UptrendMinDominance)
	}

	spy := &spySender{}
	eng := notifications.NewEngine(spy, notifications.DefaultEngineConfig())
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })

	cfgStore := newMemConfigStore()
	_ = cfgStore.Save(notifications.NotificationConfig{
		UserID:              "u1",
		Uptrend:             true,
		UptrendMinDominance: notifications.DefaultNotificationConfig("u1").UptrendMinDominance,
		UptrendTimeframe:    "1h",
	})

	sched := notifications.NewScheduler(eng, market, nil, nil, notifications.DefaultSchedulerConfig())
	sched.SetConfigStore(cfgStore)
	sched.SetClock(func() time.Time { return now })
	sched.CheckMarketState(context.Background())

	if spy.userCount() != 1 {
		t.Fatalf("expected the realistic strong-trend market to fire an Uptrend notification, got %d notifications", spy.userCount())
	}
}
