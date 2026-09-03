package social

import domain "pano_chart/backend/domain/social"

// Dispatcher delivers newly discovered posts to consumers.
// For MVP this is a buffered channel; future implementations may push to
// Redis pub/sub, Kafka, or WebSocket broadcast.
type Dispatcher struct {
	ch chan []domain.Post
}

// NewDispatcher creates a dispatcher with the given buffer size.
func NewDispatcher(bufSize int) *Dispatcher {
	return &Dispatcher{ch: make(chan []domain.Post, bufSize)}
}

// Dispatch sends a batch of new posts. Non-blocking: drops if buffer full.
func (d *Dispatcher) Dispatch(posts []domain.Post) {
	select {
	case d.ch <- posts:
	default:
		// buffer full — drop (consumer too slow)
	}
}

// Events returns the read-only channel for consumers.
func (d *Dispatcher) Events() <-chan []domain.Post {
	return d.ch
}
