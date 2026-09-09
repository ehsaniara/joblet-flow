package store

import (
	"context"
	"log/slog"
	"net"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// Shipper is a pub/sub subscriber that forwards events over the flow-store Unix
// socket. It dials lazily and reconnects on failure; events published while
// disconnected are dropped for durability (recovered by a later checkpoint),
// never blocking the engine.
type Shipper struct {
	socket string
	events <-chan *storepb.StoreEvent
	log    *slog.Logger
}

// NewShipper subscribes to ps and ships its events to the socket.
func NewShipper(socket string, ps *PubSub, log *slog.Logger) (*Shipper, func()) {
	if log == nil {
		log = slog.Default()
	}
	events, unsub := ps.Subscribe()
	return &Shipper{socket: socket, events: events, log: log}, unsub
}

// Run ships events until ctx is cancelled or the subscription closes.
func (s *Shipper) Run(ctx context.Context) {
	var conn net.Conn
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.events:
			if !ok {
				return
			}
			if conn == nil {
				c, err := net.Dial("unix", s.socket)
				if err != nil {
					s.log.Warn("flow-store unreachable, dropping event", "error", err)
					continue
				}
				conn = c
			}
			if err := writeEvent(conn, ev); err != nil {
				s.log.Warn("flow-store write failed, will reconnect", "error", err)
				conn.Close()
				conn = nil
			}
		}
	}
}
