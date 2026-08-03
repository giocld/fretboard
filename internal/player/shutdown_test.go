package player

import (
	"testing"
)

func TestEngineShutdownStopsPlayback(t *testing.T) {
	e := NewEngine()
	e.shutdown = true
	if !e.ShutdownRequested() {
		t.Fatal("expected shutdown flag")
	}
	if err := e.Play(nil, 120, PlayContext{}); err == nil {
		t.Fatal("expected play to fail after shutdown")
	}
}
