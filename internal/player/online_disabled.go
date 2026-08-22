//go:build noytdlp

package player

import (
	"errors"

	"fretboard/internal/model"
)

// OnlineAudioEnabled reports whether the online-audio feature (yt-dlp
// search, ranking, download) is compiled in. With -tags noytdlp the feature
// is compiled out; callers must guard on this constant (the Wave 2
// integration wires the remaining call sites).
const OnlineAudioEnabled = false

// Stubs below keep the rest of the package compiling when audio_online.go
// is excluded. Each returns the safe "feature absent" value so runtime
// behavior degrades to local-only audio instead of crashing.

// OnlineAudioAvailable always reports false: no yt-dlp integration exists
// in this build.
func OnlineAudioAvailable() bool { return false }

// AudioSearchQuery returns an empty query: nothing to search for.
func AudioSearchQuery(tab *model.Tab) string { return "" }

// SearchOnlineCandidates returns no candidates.
func SearchOnlineCandidates(tab *model.Tab, limit int) ([]AudioSource, error) {
	return nil, nil
}

// ResolveAudio never resolves online audio; local-file resolution is left
// to the caller's local paths.
func ResolveAudio(tab *model.Tab, tabPath string, extraDirs []string, allowOnline bool) (string, error) {
	return "", nil
}

// EnsureAudioSource reports that online audio is unavailable in this build.
func EnsureAudioSource(tab *model.Tab, src AudioSource) (string, error) {
	return "", errors.New("online audio is disabled (built with -tags noytdlp)")
}
