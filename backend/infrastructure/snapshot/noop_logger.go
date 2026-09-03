package snapshot

import "pano_chart/backend/domain"

// NoopLogger silently discards all snapshots.
// Use as a safe default when no persistence is configured.
type NoopLogger struct{}

// Log implements ports.SnapshotLogger.
func (n *NoopLogger) Log(_ domain.EvaluationSnapshot) error {
	return nil
}
