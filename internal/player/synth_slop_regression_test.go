package player

import (
	"strings"
	"testing"
)

// TestSummarizeStderrPinsFiltering guards summarizeStderr's line-filtering
// contract: only error-ish lines survive, empty input maps to empty output,
// and the tail is kept when nothing matches.
func TestSummarizeStderrPinsFiltering(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"keeps_error_lines", "alsa error: pcm not found\ninfo: loaded\njack failed to start\n", "alsa error: pcm not found; jack failed to start"},
		{"no_useful_lines_short", "line one\nline two\n", "line one\nline two\n"},
		{"no_useful_lines_long_keeps_tail", "1\n2\n3\n4\n5\n6\n7\n8", "3\n4\n5\n6\n7\n8"},
		{"trims_whitespace", "  error: boom  \n", "error: boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeStderr(tc.in); got != tc.want {
				t.Fatalf("summarizeStderr(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSummarizeStderrCapsUsefulLines guards the cap: with more than four
// useful lines only the last four survive.
func TestSummarizeStderrCapsUsefulLines(t *testing.T) {
	msg := "error one\nerror two\nerror three\nfailed four\ncannot five\nerror six"
	got := summarizeStderr(msg)
	if strings.Contains(got, "error one") || strings.Contains(got, "error two") {
		t.Fatalf("first useful lines should be dropped, got %q", got)
	}
	if !strings.Contains(got, "error three") || !strings.Contains(got, "error six") {
		t.Fatalf("last four useful lines must survive, got %q", got)
	}
}

// TestFluidsynthArgsPinsShape guards fluidsynthArgs: the default driver omits
// -a, an explicit driver carries -a, and the gain/rate/soundfont/mid args are
// always present.
func TestFluidsynthArgsPinsShape(t *testing.T) {
	def := fluidsynthArgs("default", "1.60", "/sf.sf2", "/tmp/x.mid")
	if containsArg(def, "-a") {
		t.Fatalf("default driver must not pass -a: %v", def)
	}
	for _, want := range []string{"-ni", "-q", "-g", "1.60", "-r", "44100", "/sf.sf2", "/tmp/x.mid"} {
		if !containsArg(def, want) {
			t.Fatalf("default args missing %q: %v", want, def)
		}
	}

	exp := fluidsynthArgs("pulseaudio", "1.60", "/sf.sf2", "/tmp/x.mid")
	if !containsArg(exp, "-a") || !containsArg(exp, "pulseaudio") {
		t.Fatalf("explicit driver must pass -a <driver>: %v", exp)
	}
}
