package player

import "testing"

func TestStderrCollectorDoesNotBlock(t *testing.T) {
	var c stderrCollector
	n, err := c.Write(make([]byte, 256*1024))
	if err != nil || n != 256*1024 {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
	if c.String() == "" {
		t.Fatal("expected captured stderr")
	}
}
