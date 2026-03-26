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

// MarketProvider returns the current market state for a timeframe.
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

	for _, ev := range events {
		diff := ev.Timestamp().Sub(now)
		if diff < 0 || diff > s.cfg.MacroLeadTime+time.Minute {
			continue
		}

		minutes := int(diff.Minutes())
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

	// Per-user path: pre-compute breadth for each needed timeframe,
	// then evaluate each user's regime thresholds per their chosen timeframe.
	if s.configs != nil {
		configs, err := s.configs.All()
		if err != nil {
			log.Printf("[notify-scheduler] fetch configs error: %v", err)
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
		if summary.Label != "" {
			msg = summary.Label
		} else {
			msg = "Market trending today"
		}
	case mkt.StateCompression:
		msg = "Market in compression — expansion likely"
	case mkt.StateBreakout:
		msg = "Market breakout in progress"
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

// normalizeBreadth returns the fraction of a single breadth value relative
// to the sum of all four breadth components.  This converts the raw breadth
// (which may not sum to exactly 1.0 depending on the upstream scoring) into
// an intuitive percentage that matches the "prevalence" shown on screen.
func normalizeBreadth(b mkt.Breadth, value float64) float64 {
	total := b.Trend + b.Sideways + b.Compression + b.Breakout
	if total == 0 {
		return 0
	}
	return value / total
}

// checkMarketForUser evaluates the market breadth against a single user's
// enabled regimes and thresholds. Each regime may reference a different
// timeframe. The strongest qualifying regime wins.
func (s *Scheduler) checkMarketForUser(ctx context.Context, cfg NotificationConfig, summaries map[string]mkt.Summary) {
	if !s.userHasProAccess(ctx, cfg.UserID) {
		return
	}

	type candidate struct {
		label      string
		prevalence float64
		timeframe  string
	}

	var candidates []candidate

	// Uptrend — maps to Breadth.Trend.
	if cfg.Uptrend {
		if sum, ok := summaries[cfg.UptrendTimeframe]; ok {
			p := normalizeBreadth(sum.Breadth, sum.Breadth.Trend)
			if p >= cfg.UptrendMinDominance {
				lbl := "Uptrend"
				if sum.Label != "" {
					lbl = sum.Label
				}
				candidates = append(candidates, candidate{lbl, p, cfg.UptrendTimeframe})
			}
		}
	}
	// Downtrend — maps to Breadth.Breakout.
	if cfg.Downtrend {
		if sum, ok := summaries[cfg.DowntrendTimeframe]; ok {
			p := normalizeBreadth(sum.Breadth, sum.Breadth.Breakout)
			if p >= cfg.DowntrendMinDominance {
				candidates = append(candidates, candidate{"Downtrend", p, cfg.DowntrendTimeframe})
			}
		}
	}
	// Sideways — maps to Breadth.Sideways.
	if cfg.Sideways {
		if sum, ok := summaries[cfg.SidewaysTimeframe]; ok {
			p := normalizeBreadth(sum.Breadth, sum.Breadth.Sideways)
			if p >= cfg.SidewaysMinDominance {
				candidates = append(candidates, candidate{"Sideways", p, cfg.SidewaysTimeframe})
			}
		}
	}

	if len(candidates) == 0 {
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
			setups[tf] = best
		}

		dateKey := s.now().Format("2006-01-02")
		for _, cfg := range configs {
			if !cfg.SetupOfDay {
				continue
			}
			if !s.userHasProAccess(ctx, cfg.UserID) {
				continue
			}
			best, ok := setups[cfg.SetupTimeframe]
			if !ok || best.Score < cfg.SetupMinScore {
				continue
			}
			// Health gate: skip trend-based setups with weak trend health.
			if best.BestSetup == setup.TrendContinuation && best.TrendHealth < 0.4 {
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

	// Health gate: skip trend-based setups with weak trend health.
	if best.BestSetup == setup.TrendContinuation && best.TrendHealth < 0.4 {
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
