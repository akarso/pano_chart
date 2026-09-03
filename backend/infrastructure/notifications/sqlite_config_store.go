package notifications

import (
	"database/sql"
	"encoding/json"
	"fmt"

	appnotify "pano_chart/backend/application/notifications"
)

// Compile-time check.
var _ appnotify.NotificationConfigStore = (*SQLiteConfigStore)(nil)

// SQLiteConfigStore implements NotificationConfigStore backed by SQLite.
type SQLiteConfigStore struct {
	db *sql.DB
}

// NewSQLiteConfigStore creates or opens a config store.
func NewSQLiteConfigStore(db *sql.DB) (*SQLiteConfigStore, error) {
	s := &SQLiteConfigStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate notification_config: %w", err)
	}
	return s, nil
}

func (s *SQLiteConfigStore) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS notification_config (
		user_id    TEXT PRIMARY KEY,
		config     TEXT NOT NULL,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

// configJSON is the serialised form stored in the config column.
type configJSON struct {
	Social                bool    `json:"social"`
	Macro                 *bool   `json:"macro,omitempty"`
	MacroHigh             bool    `json:"macro_high"`
	MacroModerate         bool    `json:"macro_moderate"`
	News                  bool    `json:"news"`
	Uptrend               bool    `json:"uptrend"`
	Downtrend             bool    `json:"downtrend"`
	Sideways              bool    `json:"sideways"`
	SetupOfDay            bool    `json:"setup_of_day"`
	UptrendMinDominance   float64 `json:"uptrend_min_dominance"`
	DowntrendMinDominance float64 `json:"downtrend_min_dominance"`
	SidewaysMinDominance  float64 `json:"sideways_min_dominance"`
	SetupMinScore         float64 `json:"setup_min_score"`
	UptrendTimeframe      string  `json:"uptrend_timeframe,omitempty"`
	DowntrendTimeframe    string  `json:"downtrend_timeframe,omitempty"`
	SidewaysTimeframe     string  `json:"sideways_timeframe,omitempty"`
	SetupTimeframe        string  `json:"setup_timeframe,omitempty"`
}

func toJSON(cfg appnotify.NotificationConfig) configJSON {
	return configJSON{
		Social:                cfg.Social,
		MacroHigh:             cfg.MacroHigh,
		MacroModerate:         cfg.MacroModerate,
		News:                  cfg.News,
		Uptrend:               cfg.Uptrend,
		Downtrend:             cfg.Downtrend,
		Sideways:              cfg.Sideways,
		SetupOfDay:            cfg.SetupOfDay,
		UptrendMinDominance:   cfg.UptrendMinDominance,
		DowntrendMinDominance: cfg.DowntrendMinDominance,
		SidewaysMinDominance:  cfg.SidewaysMinDominance,
		SetupMinScore:         cfg.SetupMinScore,
		UptrendTimeframe:      cfg.UptrendTimeframe,
		DowntrendTimeframe:    cfg.DowntrendTimeframe,
		SidewaysTimeframe:     cfg.SidewaysTimeframe,
		SetupTimeframe:        cfg.SetupTimeframe,
	}
}

func fromJSON(userID string, j configJSON) appnotify.NotificationConfig {
	// Migrate legacy "macro" bool → split fields.
	macroHigh := j.MacroHigh
	macroMod := j.MacroModerate
	if j.Macro != nil {
		// Old config had single bool — apply to both.
		macroHigh = *j.Macro
		macroMod = *j.Macro
	}
	cfg := appnotify.NotificationConfig{
		UserID:                userID,
		Social:                j.Social,
		MacroHigh:             macroHigh,
		MacroModerate:         macroMod,
		News:                  j.News,
		Uptrend:               j.Uptrend,
		Downtrend:             j.Downtrend,
		Sideways:              j.Sideways,
		SetupOfDay:            j.SetupOfDay,
		UptrendMinDominance:   j.UptrendMinDominance,
		DowntrendMinDominance: j.DowntrendMinDominance,
		SidewaysMinDominance:  j.SidewaysMinDominance,
		SetupMinScore:         j.SetupMinScore,
		UptrendTimeframe:      j.UptrendTimeframe,
		DowntrendTimeframe:    j.DowntrendTimeframe,
		SidewaysTimeframe:     j.SidewaysTimeframe,
		SetupTimeframe:        j.SetupTimeframe,
	}
	// Backfill empty timeframes from pre-existing configs.
	if cfg.UptrendTimeframe == "" {
		cfg.UptrendTimeframe = "1h"
	}
	if cfg.DowntrendTimeframe == "" {
		cfg.DowntrendTimeframe = "1h"
	}
	if cfg.SidewaysTimeframe == "" {
		cfg.SidewaysTimeframe = "1h"
	}
	if cfg.SetupTimeframe == "" {
		cfg.SetupTimeframe = "1h"
	}
	return cfg
}

// Get returns the config for a user, or defaults if none saved.
func (s *SQLiteConfigStore) Get(userID string) (appnotify.NotificationConfig, error) {
	var raw string
	err := s.db.QueryRow(`SELECT config FROM notification_config WHERE user_id = ?`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return appnotify.DefaultNotificationConfig(userID), nil
	}
	if err != nil {
		return appnotify.NotificationConfig{}, fmt.Errorf("query config: %w", err)
	}

	var j configJSON
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return appnotify.NotificationConfig{}, fmt.Errorf("unmarshal config: %w", err)
	}
	return fromJSON(userID, j), nil
}

// Save creates or updates the config for a user.
func (s *SQLiteConfigStore) Save(cfg appnotify.NotificationConfig) error {
	data, err := json.Marshal(toJSON(cfg))
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, err = s.db.Exec(`INSERT INTO notification_config (user_id, config, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(user_id) DO UPDATE SET
			config     = excluded.config,
			updated_at = datetime('now')`,
		cfg.UserID, string(data),
	)
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// All returns configs for every user that has one saved.
func (s *SQLiteConfigStore) All() ([]appnotify.NotificationConfig, error) {
	rows, err := s.db.Query(`SELECT user_id, config FROM notification_config`)
	if err != nil {
		return nil, fmt.Errorf("query all configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var configs []appnotify.NotificationConfig
	for rows.Next() {
		var userID, raw string
		if err := rows.Scan(&userID, &raw); err != nil {
			return nil, fmt.Errorf("scan config: %w", err)
		}
		var j configJSON
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			return nil, fmt.Errorf("unmarshal config for %s: %w", userID, err)
		}
		configs = append(configs, fromJSON(userID, j))
	}
	return configs, rows.Err()
}
