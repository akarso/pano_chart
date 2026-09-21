package ports

import (
	"context"

	"pano_chart/backend/domain/signal"
)

// SignalEmitter records a signal best-effort (dedupe + persist). Nil-safe
// implementers no-op. See ROADMAP PR-090.
//
// Returns true when the signal is accounted for (freshly persisted or already
// present for this candle). Returns false when persistence failed, the write
// was skipped, or a peer is already in flight for the same dedupe key —
// callers that maintain change-detection state must only advance on true.
type SignalEmitter interface {
	Emit(ctx context.Context, s signal.Signal) bool
}
