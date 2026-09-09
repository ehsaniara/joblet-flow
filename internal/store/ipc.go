package store

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"google.golang.org/protobuf/proto"

	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

// maxEventSize caps a single framed event, guarding the reader against a
// corrupt length prefix.
const maxEventSize = 8 << 20 // 8 MiB

// writeEvent frames ev as a 4-byte big-endian length prefix followed by the
// marshaled protobuf.
func writeEvent(w io.Writer, ev *storepb.StoreEvent) error {
	b, err := proto.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// readEvent reads one length-prefixed event.
func readEvent(r io.Reader) (*storepb.StoreEvent, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxEventSize {
		return nil, fmt.Errorf("event too large: %d bytes", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	ev := &storepb.StoreEvent{}
	if err := proto.Unmarshal(buf, ev); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return ev, nil
}

// ServeConn reads framed events off conn until EOF, applying each to backend.
// One connection is expected (flow-core to flow-store); it returns when the
// peer closes or a read fails.
func ServeConn(conn net.Conn, backend Backend, apply func(*storepb.StoreEvent) error) error {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		ev, err := readEvent(r)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := apply(ev); err != nil {
			return fmt.Errorf("apply event: %w", err)
		}
	}
}
