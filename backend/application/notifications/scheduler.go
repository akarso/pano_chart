package notifications

import (
	"context"
	"fmt"
	"log"
	"sync"
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
	MarketMinConfidence float64       // minimum dominance to notify
	SetupCheckInterval  time.Duration // how often to check best setup
	SetupMinScore       float64       // minimum score to notify
	Timeframe           string        // timeframe for market + setup checks
}

// DefaultSchedulerConfig returns production defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MacroCheckInterval:  1 * time.Minute,
		MacroLeadTime:       30 * time.Minute,
		MarketCheckInterval: 1 * time.Hour,
		MarketMinConfidence: 0.75,
		SetupCheckInterval:  1 * time.Hour,
		SetupMinScore:       0.75,
		Timeframe:           "1h",
	}
}

// ---------- scheduler ----------

// Scheduler periodically checks data sources and sends notifications
// through the Engine.
type Scheduler struct {
	engine   *Engine
	market   MarketProvider  // optional
	setups   SetupProvider   // optional
	events   EventProvider   // optional
	cfg      SchedulerConfig
	lastRun  map[string]time.Time
	lastRunM sync.Mutex
	now      func() time.Time
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
		engine:  engine,
		market:  market,
		setups:  setups,
		events:  events,
		cfg:     cfg,
		lastRun: make(map[string]time.Time),
		now:     time.Now,
	}
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

// CheckMarketState evaluates market dominance and notifies once per day.
func (s *Scheduler) CheckMarketState(ctx context.Context) {
	s.checkMarketState(ctx)
}

// CheckSetupOfDay evaluates the best setup and notifies once per day.
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
			Key:   "macro_" + ev.ID(),
		})
	}
}

func (s *Scheduler) checkMarketState(ctx context.Context) {
	if s.market == nil {
		return
	}
	if !s.oncePerDay("market_state") {
		return
	}

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
		msg = "Market trending today"
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
		Key:   fmt.Sprintf("market_%s_%s", summary.Timeframe, summary.State),
	})
}

func (s *Scheduler) checkSetupOfDay(ctx context.Context) {
	if s.setups == nil {
		return
	}
	if !s.oncePerDay("setup_of_day") {
		return
	}

	best, err := s.setups.BestSetup(ctx, s.cfg.Timeframe)
	if err != nil {
		log.Printf("[notify-scheduler] best setup error: %v", err)
		return
	}

	if best.Score < s.cfg.SetupMinScore {
		return
	}

	body := fmt.Sprintf("%s (%0.f%%)", best.Symbol, best.Score*100)

	_ = s.engine.Send(ctx, Notification{
		Type:  TypeSetup,
		Title: "Setup of the Day",
		Body:  body,
		Key:   fmt.Sprintf("setup_%s_%s", best.Symbol, s.now().Format("2006-01-02")),
	})
}

// oncePerDay returns true at most once per 24h per key.
func (s *Scheduler) oncePerDay(key string) bool {
	s.lastRunM.Lock()
	defer s.lastRunM.Unlock()

	now := s.now()
	if last, ok := s.lastRun[key]; ok && now.Sub(last) < 24*time.Hour {
		return false
	}
	s.lastRun[key] = now
	return true
}

// SetClock overrides the time source (for testing).
func (s *Scheduler) SetClock(fn func() time.Time) { s.now = fn }
