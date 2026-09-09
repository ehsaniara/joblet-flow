// Package proto holds the internal flow-core <-> flow-store IPC framing.
// It is private to joblet-flow and is not published in joblet-proto.
package proto

//go:generate protoc --go_out=gen --go_opt=paths=source_relative store.proto
