package notifications

import (
	"context"
	"fmt"
	"log"
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/setup"
)

// ---------- provider ports ----------

// MarketProvider returns the current market summary for a timeframe.
type MarketProvider interface {
	Calculate(timeframe string) (mkt.Summary, error)
}

// SetupProvider returns the best setup for a timeframe.
type SetupProvider interface {
	BestSetup(ctx context.Context, timeframe string) (setup.SetupScores, error)
}

// EventProvider returns events within a date range.
type EventProvider interface {
	FetchEvents(ctx context.Context, from, to time.Time) ([]domain.Event, error)
}

// ---------- scheduler config ----------

// SchedulerConfig holds cadence and threshold tunables.
type SchedulerConfig struct {
	MacroCheckInterval  time.Duration // how often to scan for upcoming events
	MacroLeadTime       time.Duration // alert this far ahead of an event
	MarketCheckInterval time.Duration // how often to check market state
	MarketMinConfidence float64       // minimum dominance to notify (legacy)
	SetupCheckInterval  time.Duration // how often to check best setup
	SetupMinScore       float64       // minimum score to notify (legacy)
	Timeframe           string        // fallback timeframe for legacy broadcast
}

// DefaultSchedulerConfig returns production defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MacroCheckInterval:  1 * time.Minute,
		MacroLeadTime:       30 * time.Minute,
		MarketCheckInterval: 1 * time.Minute,
		MarketMinConfidence: 0.75,
		SetupCheckInterval:  1 * time.Minute,
		SetupMinScore:       0.75,
		Timeframe:           "1h",
	}
}

// ---------- scheduler ----------

// Scheduler periodically checks data sources and sends notifications
// through the Engine.
type Scheduler struct {
	engine        *Engine
	market        MarketProvider          // optional
	setups        SetupProvider           // optional
	events        EventProvider           // optional
	configs       NotificationConfigStore // optional — enables per-user thresholds
	subscriptions SubscriptionChecker     // optional — gates pro-only notifications
	cfg           SchedulerConfig
	now           func() time.Time
}

// NewScheduler creates the scheduler. Pass nil for any provider to skip that check.
func NewScheduler(
	engine *Engine,
	market MarketProvider,
	setups SetupProvider,
	events EventProvider,
	cfg SchedulerConfig,
) *Scheduler {
	return &Scheduler{
		engine: engine,
		market: market,
		setups: setups,
		events: events,
		cfg:    cfg,
		now:    time.Now,
	}
}

// SetConfigStore attaches a per-user notification config store.
// When set, market and setup notifications use per-user thresholds.
func (s *Scheduler) SetConfigStore(store NotificationConfigStore) {
	s.configs = store
}

// SetSubscriptionChecker attaches a subscription checker.
// When set, pro-only notification types (market, setup, macro) are only
// delivered to users with an active subscription. News notifications
// remain available to all users.
func (s *Scheduler) SetSubscriptionChecker(checker SubscriptionChecker) {
	s.subscriptions = checker
}

// Run starts the scheduler loops. Blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	macroTicker := time.NewTicker(s.cfg.MacroCheckInterval)
	defer macroTicker.Stop()

	marketTicker := time.NewTicker(s.cfg.MarketCheckInterval)
	defer marketTicker.Stop()

	setupTicker := time.NewTicker(s.cfg.SetupCheckInterval)
	defer setupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-macroTicker.C:
			s.checkMacroEvents(ctx)
		case <-marketTicker.C:
			s.checkMarketState(ctx)
		case <-setupTicker.C:
			s.checkSetupOfDay(ctx)
		}
	}
}

// ---------- check methods (exported for direct / test invocation) ----------

// CheckMacroEvents scans events and notifies for any approaching within the lead time.
func (s *Scheduler) CheckMacroEvents(ctx context.Context) {
	s.checkMacroEvents(ctx)
}

// CheckMarketState evaluates market dominance and notifies.
func (s *Scheduler) CheckMarketState(ctx context.Context) {
	s.checkMarketState(ctx)
}

// CheckSetupOfDay evaluates the best setup and notifies.
func (s *Scheduler) CheckSetupOfDay(ctx context.Context) {
	s.checkSetupOfDay(ctx)
}

// ---------- internals ----------

func (s *Scheduler) checkMacroEvents(ctx context.Context) {
	if s.events == nil {
		return
	}

	now := s.now()
	from := now
	to := now.Add(s.cfg.MacroLeadTime + 5*time.Minute)

	events, err := s.events.FetchEvents(ctx, from, to)
	if err != nil {
		log.Printf("[notify-scheduler] macro events fetch error: %v", err)
		return
	}

	var upcoming []domain.Event
	for _, ev := range events {
		diff := ev.Timestamp().Sub(now)
		if diff < 0 || diff > s.cfg.MacroLeadTime+time.Minute {
			continue
		}
		upcoming = append(upcoming, ev)
	}

	if len(upcoming) == 0 {
		return
	}

	// Per-user path: filter events by impact preference.
	if s.configs != nil {
		configs, err := s.configs.All()
		if err != nil {
			log.Printf("[notify-scheduler] fetch configs error: %v", err)
			return
		}

		for _, ev := range upcoming {
			minutes := int(ev.Timestamp().Sub(now).Minutes())
			body := fmt.Sprintf("%s in ~%d minutes", ev.Title(), minutes)
			n := Notification{
				Type:  TypeMacro,
				Title: "Upcoming Macro Event",
				Body:  body,
				Data:  map[string]string{"type": string(TypeMacro)},
				Key:   "macro_" + ev.ID(),
			}

			for _, cfg := range configs {
				if !s.userWantsMacro(cfg, ev.Impact()) {
					continue
				}
				if !s.userHasProAccess(ctx, cfg.UserID) {
					continue
				}
				_ = s.engine.SendToUser(ctx, cfg.UserID, n)
			}
		}
		return
	}

	// Legacy broadcast path (no config store).
	for _, ev := range upcoming {
		minutes := int(ev.Timestamp().Sub(now).Minutes())
		body := fmt.Sprintf("%s in ~%d minutes", ev.Title(), minutes)

		_ = s.engine.Send(ctx, Notification{
			Type:  TypeMacro,
			Title: "Upcoming Macro Event",
			Body:  body,
			Data:  map[string]string{"type": string(TypeMacro)},
			Key:   "macro_" + ev.ID(),
		})
	}
}

// userWantsMacro returns true if the user's config allows notifications for
// the given event impact level.
func (s *Scheduler) userWantsMacro(cfg NotificationConfig, impact domain.EventImpact) bool {
	switch impact {
	case domain.EventImpactHigh:
		return cfg.MacroHigh
	case domain.EventImpactMedium:
		return cfg.MacroModerate
	default:
		return false // low-impact events never trigger notifications
	}
}

// collectTimeframes returns the distinct set of timeframes needed from
// the user configs. Each regime and setup may use a different timeframe.
func collectTimeframes(configs []NotificationConfig) map[string]struct{} {
	tfs := map[string]struct{}{}
	for _, cfg := range configs {
		if cfg.Uptrend {
			tfs[cfg.UptrendTimeframe] = struct{}{}
		}
		if cfg.Downtrend {
			tfs[cfg.DowntrendTimeframe] = struct{}{}
		}
		if cfg.Sideways {
			tfs[cfg.SidewaysTimeframe] = struct{}{}
		}
		if cfg.SetupOfDay {
			tfs[cfg.SetupTimeframe] = struct{}{}
		}
	}
	return tfs
}

func (s *Scheduler) checkMarketState(ctx context.Context) {
	if s.market == nil {
		return
	}

	// Per-user path: pre-compute regime for each needed timeframe,
	// then evaluate each user's score thresholds per their chosen timeframe.
	if s.configs != nil {
		configs, err := s.configs.All()
		if err != nil {
			log.Printf("[notify-scheduler] fetch configs error: %v", err)
			return
		}

		if len(configs) == 0 {
			log.Printf("[notify-scheduler] market: no user configs found — skipping")
			return
		}

		tfs := collectTimeframes(configs)
		summaries := make(map[string]mkt.Summary, len(tfs))
		for tf := range tfs {
			summary, err := s.market.Calculate(tf)
			if err != nil {
				log.Printf("[notify-scheduler] market calc %s error: %v", tf, err)
				continue
			}
			log.Printf("[notify-scheduler] market %s: regime=%s bias=%s prevalence=%.2f scores={trend=%.2f sideways=%.2f compression=%.2f expansion=%.2f}",
				tf, summary.State, summary.Bias, summary.Confidence,
				summary.Breadth.Trend, summary.Breadth.Sideways,
				summary.Breadth.Compression, summary.Breadth.Expansion)
			summaries[tf] = summary
		}

		for _, cfg := range configs {
			s.checkMarketForUser(ctx, cfg, summaries)
		}
		return
	}

	// Legacy broadcast path (no config store).
	summary, err := s.market.Calculate(s.cfg.Timeframe)
	if err != nil {
		log.Printf("[notify-scheduler] market state error: %v", err)
		return
	}

	if summary.Confidence < s.cfg.MarketMinConfidence {
		return
	}

	var msg string
	switch summary.State {
	case mkt.StateSideways:
		msg = "Market mostly sideways today"
	case mkt.StateTrend:
		switch {
		case summary.Label != "":
			msg = summary.Label
		case summary.Bias == "up":
			msg = "Market trending up today"
		case summary.Bias == "down":
			msg = "Market trending down today"
		default:
			msg = "Market trending today"
		}
	case mkt.StateCompression:
		msg = "Market in compression — expansion likely"
	case mkt.StateExpansion:
		msg = "Market expansion in progress"
	case mkt.StateSilent:
		msg = "Market is quiet — low activity"
	case mkt.StateIndecisive:
		// Do not push for indecisive — nothing actionable.
		return
	default:
		msg = "Market regime: " + string(summary.State)
	}

	_ = s.engine.Send(ctx, Notification{
		Type:  TypeMarket,
		Title: "Market Update",
		Body:  msg,
		Data:  map[string]string{"type": string(TypeMarket)},
		Key:   fmt.Sprintf("market_%s_%s", summary.Timeframe, summary.State),
	})
}

// checkMarketForUser evaluates the market breadth against a single user's
// enabled regimes and thresholds. Each regime may reference a different
// timeframe. The strongest qualifying regime wins.
func (s *Scheduler) checkMarketForUser(ctx context.Context, cfg NotificationConfig, summaries map[string]mkt.Summary) {
	if !s.userHasProAccess(ctx, cfg.UserID) {
		log.Printf("[notify-scheduler] market: user=%s blocked by subscription check", cfg.UserID)
		return
	}

	type candidate struct {
		label      string
		prevalence float64
		timeframe  string
	}

	var candidates []candidate

	// Uptrend — maps to Breadth.Trend, gated on the regime's actual
	// direction (sum.Bias, an aggregate-return-based signal computed by
	// MarketStateService.Calculate — see PR-072). Skip if the regime is
	// indecisive — nothing actionable.
	if cfg.Uptrend {
		if sum, ok := summaries[cfg.UptrendTimeframe]; ok &&
			sum.State != mkt.StateIndecisive && sum.Bias == "up" {
			p := sum.Breadth.Trend
			if p >= cfg.UptrendMinDominance {
				// Always "Uptrend", never sum.Label (e.g. "Strong trend") —
				// BuildMarketLabel's labels are direction-agnostic and would
				// obscure the very bullish/bearish distinction this
				// notification exists to convey.
				candidates = append(candidates, candidate{"Uptrend", p, cfg.UptrendTimeframe})
			}
		}
	}
	// Downtrend — maps to Breadth.Trend gated on sum.Bias == "down". This
	// used to read Breadth.Expansion (breakout/expansion activity, nothing
	// to do with direction) — a "Downtrend" subscriber was getting
	// breakout alerts and never anything about actual declines. Fixed in
	// PR-072.
	//
	// Side effect: existing users' DowntrendMinDominance values were tuned
	// against Expansion score magnitudes, which have a different
	// distribution than Trend scores. SQLiteConfigStore.fromJSON resets this
	// field to the default for any config stored before this change
	// (config_version < 1) rather than silently reusing a threshold tuned
	// for a different metric — see infrastructure/notifications/sqlite_config_store.go.
	if cfg.Downtrend {
		if sum, ok := summaries[cfg.DowntrendTimeframe]; ok &&
			sum.State != mkt.StateIndecisive && sum.Bias == "down" {
			p := sum.Breadth.Trend
			if p >= cfg.DowntrendMinDominance {
				// Always "Downtrend" — see the Uptrend branch above for why
				// sum.Label must not be used here.
				candidates = append(candidates, candidate{"Downtrend", p, cfg.DowntrendTimeframe})
			}
		}
	}
	// Sideways — maps to Breadth.Sideways.
	if cfg.Sideways {
		if sum, ok := summaries[cfg.SidewaysTimeframe]; ok && sum.State != mkt.StateIndecisive {
			lbl := "Sideways"
			if sum.State == mkt.StateSilent {
				lbl = "Silent"
			}
			p := sum.Breadth.Sideways
			if p >= cfg.SidewaysMinDominance {
				candidates = append(candidates, candidate{lbl, p, cfg.SidewaysTimeframe})
			}
		}
	}

	if len(candidates) == 0 {
		log.Printf("[notify-scheduler] market: user=%s no regime exceeds threshold (uptrend=%v/%.0f%% downtrend=%v/%.0f%% sideways=%v/%.0f%%)",
			cfg.UserID,
			cfg.Uptrend, cfg.UptrendMinDominance*100,
			cfg.Downtrend, cfg.DowntrendMinDominance*100,
			cfg.Sideways, cfg.SidewaysMinDominance*100)
		return
	}

	// Pick the strongest.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.prevalence > best.prevalence {
			best = c
		}
	}

	body := fmt.Sprintf("Market is %s (%.0f%%, %s)", best.label, best.prevalence*100, best.timeframe)
	dateKey := s.now().Format("2006-01-02")

	_ = s.engine.SendToUser(ctx, cfg.UserID, Notification{
		Type:  TypeMarket,
		Title: "Market Update",
		Body:  body,
		Data:  map[string]string{"type": string(TypeMarket), "timeframe": best.timeframe},
		Key:   fmt.Sprintf("market_%s_%s", best.timeframe, dateKey),
	})
}

func (s *Scheduler) checkSetupOfDay(ctx context.Context) {
	if s.setups == nil {
		return
	}

	// Per-user path: compute best setup per needed timeframe.
	if s.configs != nil {
		configs, err := s.configs.All()
		if err != nil {
			log.Printf("[notify-scheduler] fetch configs for setup error: %v", err)
			return
		}

		// Collect distinct setup timeframes.
		tfs := map[string]struct{}{}
		for _, cfg := range configs {
			if cfg.SetupOfDay {
				tfs[cfg.SetupTimeframe] = struct{}{}
			}
		}

		setups := make(map[string]setup.SetupScores, len(tfs))
		for tf := range tfs {
			best, err := s.setups.BestSetup(ctx, tf)
			if err != nil {
				log.Printf("[notify-scheduler] best setup %s error: %v", tf, err)
				continue
			}
			log.Printf("[notify-scheduler] setup %s: best=%s score=%.2f confidence=%.2f",
				tf, best.Symbol, best.Score, best.Confidence)
			setups[tf] = best
		}

		dateKey := s.now().Format("2006-01-02")
		for _, cfg := range configs {
			if !cfg.SetupOfDay {
				continue
			}
			if !s.userHasProAccess(ctx, cfg.UserID) {
				log.Printf("[notify-scheduler] setup: user=%s blocked by subscription check", cfg.UserID)
				continue
			}
			best, ok := setups[cfg.SetupTimeframe]
			if !ok || best.Score < cfg.SetupMinScore {
				log.Printf("[notify-scheduler] setup: user=%s score=%.2f below threshold=%.2f",
					cfg.UserID, best.Score, cfg.SetupMinScore)
				continue
			}
			// Confidence gate: only notify when contextual confidence is sufficient.
			if best.Confidence < 0.6 {
				continue
			}
			body := fmt.Sprintf("%s (%0.f%%, %s)", best.Symbol, best.Score*100, cfg.SetupTimeframe)
			_ = s.engine.SendToUser(ctx, cfg.UserID, Notification{
				Type:  TypeSetup,
				Title: "Setup of the Day",
				Body:  body,
				Data:  map[string]string{"type": string(TypeSetup), "symbol": best.Symbol, "timeframe": cfg.SetupTimeframe},
				Key:   fmt.Sprintf("setup_%s_%s_%s", best.Symbol, cfg.SetupTimeframe, dateKey),
			})
		}
		return
	}

	// Legacy broadcast path.
	best, err := s.setups.BestSetup(ctx, s.cfg.Timeframe)
	if err != nil {
		log.Printf("[notify-scheduler] best setup error: %v", err)
		return
	}

	if best.Score < s.cfg.SetupMinScore {
		return
	}

	// Confidence gate: only notify when contextual confidence is sufficient.
	if best.Confidence < 0.6 {
		return
	}

	body := fmt.Sprintf("%s (%0.f%%)", best.Symbol, best.Score*100)
	dateKey := s.now().Format("2006-01-02")

	_ = s.engine.Send(ctx, Notification{
		Type:  TypeSetup,
		Title: "Setup of the Day",
		Body:  body,
		Data:  map[string]string{"type": string(TypeSetup), "symbol": best.Symbol},
		Key:   fmt.Sprintf("setup_%s_%s", best.Symbol, dateKey),
	})
}

// userHasProAccess returns true when the user may receive pro-only
// notifications. If no SubscriptionChecker is configured, all users are
// treated as pro (backwards-compatible).
func (s *Scheduler) userHasProAccess(ctx context.Context, userID string) bool {
	if s.subscriptions == nil {
		return true
	}
	active, err := s.subscriptions.IsActive(ctx, userID)
	if err != nil {
		log.Printf("[notify-scheduler] subscription check error user=%s: %v", userID, err)
		return false // fail-closed: don't send pro notifications on errors
	}
	return active
}

// SetClock overrides the time source (for testing).
func (s *Scheduler) SetClock(fn func() time.Time) { s.now = fn }
