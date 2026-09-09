package store

import (
	"context"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// Backend persists state-change events in the flow-store subprocess. Concrete
// backends (local disk here; a third-party-backed one later) live behind this
// interface so the sink's dependencies never reach flow-core.
//
//counterfeiter:generate . Backend
type Backend interface {
	// Apply durably records one state-change event.
	Apply(ctx context.Context, ev *storepb.StoreEvent) error
	// Close flushes and releases resources.
	Close() error
}
