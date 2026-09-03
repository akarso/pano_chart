package ports

import "pano_chart/backend/domain"

// SnapshotLogger persists evaluation snapshots.
// Implementations may write to a database, file, channel, or nothing at all.
// The scoring engine must not know the storage implementation.
type SnapshotLogger interface {
	Log(snapshot domain.EvaluationSnapshot) error
}
