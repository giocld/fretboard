//go:build !noytdlp

package player

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/model"
)

// OnlineAudioAvailable reports whether yt-dlp is installed for online lookups.
func OnlineAudioAvailable() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

// AudioSearchQuery builds a search string from tab metadata.
func AudioSearchQuery(tab *model.Tab) string {
	if tab == nil {
		return ""
	}
	artist := strings.TrimSpace(tab.Artist)
	title := strings.TrimSpace(tab.Title)
	switch {
	case artist != "" && title != "":
		return artist + " " + title
	case title != "":
		return title
	case artist != "":
		return artist
	default:
		return ""
	}
}

// SearchOnlineCandidates queries YouTube and ranks matches for the tab.
func SearchOnlineCandidates(tab *model.Tab, limit int) ([]AudioSource, error) {
	if tab == nil {
		return nil, errors.New("nil tab")
	}
	if !OnlineAudioAvailable() {
		return nil, errYtDlpMissing
	}
	if limit <= 0 {
		limit = 5
	}

	// A pinned video wins unconditionally; keep every collected candidate so
	// the pin is never trimmed out of the list before the promotion pass.
	_, pinned := PinnedVideoFor(tab)

	seen := map[string]struct{}{}
	var ranked []AudioSource
	reasons := map[string]string{} // candidate ID -> pick-reason fragment
	var lastErr error

	for _, query := range AudioSearchQueries(tab) {
		entries, err := ytSearch(query, limit)
		if err != nil {
			lastErr = err
			continue
		}
		for _, e := range entries {
			if e.ID == "" {
				continue
			}
			if _, ok := seen[e.ID]; ok {
				continue
			}
			seen[e.ID] = struct{}{}
			channel := e.Channel
			if channel == "" {
				channel = e.Uploader
			}
			cat := ClassifyAudioCandidate(tab.Artist, tab.Title, e.Title, channel, e.Description)
			strictOK := StrictCompatible(cat)
			if cat == CatOther && strings.Contains(strings.ToLower(e.Title), strings.ToLower(strings.TrimSpace(tab.Title))) &&
				(strings.TrimSpace(tab.Artist) == "" || strings.Contains(strings.ToLower(e.Title), strings.ToLower(strings.TrimSpace(tab.Artist)))) {
				strictOK = true // unambiguous "Artist - Song" style title without an official marker
			}
			src, reason := onlineSourceFromEntry(tab, e, strictOK)
			reasons[src.ID] = reason
			ranked = append(ranked, src)
		}
	}

	sortAudioSources(ranked)
	if !pinned && len(ranked) > limit*2 {
		ranked = ranked[:limit*2]
	}
	// Second pass: when the primary queries found nothing (not even an
	// error), retry with the fallback phrasing — song-only engines and
	// "official audio"/"lyrics" variants often rescue the search.
	if len(ranked) == 0 {
		for _, query := range AudioSearchFallbackQueries(tab) {
			entries, err := ytSearch(query, limit)
			if err != nil {
				lastErr = err
				continue
			}
			for _, e := range entries {
				if e.ID == "" {
					continue
				}
				if _, ok := seen[e.ID]; ok {
					continue
				}
				seen[e.ID] = struct{}{}
				channel := e.Channel
				if channel == "" {
					channel = e.Uploader
				}
				cat := ClassifyAudioCandidate(tab.Artist, tab.Title, e.Title, channel, e.Description)
				strictOK := StrictCompatible(cat)
				if cat == CatOther && strings.Contains(strings.ToLower(e.Title), strings.ToLower(strings.TrimSpace(tab.Title))) {
					strictOK = true
				}
				src, reason := onlineSourceFromEntry(tab, e, strictOK)
				reasons[src.ID] = reason
				ranked = append(ranked, src)
			}
		}
		sortAudioSources(ranked)
	}

	// Pin wins forever: promote it ahead of every heuristic, synthesizing a
	// source when the search engine no longer surfaces the pinned video.
	promotePinned(tab, &ranked)

	// A total failure must not be reported as "no matches": the real cause
	// (yt-dlp missing, timed out, network error) is what the user needs.
	// A pinned video rescues this case: the user's explicit choice needs no
	// search engine.
	if len(ranked) == 0 && lastErr != nil {
		return nil, lastErr
	}

	// The top candidate gets the human-readable reason for its win.
	if len(ranked) > 0 && ranked[0].PickReason == "" {
		ranked[0].PickReason = reasons[ranked[0].ID]
		if ranked[0].PickReason == "" {
			ranked[0].PickReason = "top-ranked match"
		}
	}
	return ranked, nil
}

// maxIntScore marks a pinned candidate as unbeatable by any heuristic: the
// user's explicit choice outranks every computed score.
const maxIntScore = 1 << 30

// promotePinned re-ranks the list so a user-pinned video leads
// unconditionally. When the search no longer surfaces the pinned video, a
// minimal source is synthesized from the pin so it keeps winning.
func promotePinned(tab *model.Tab, ranked *[]AudioSource) {
	pinID, ok := PinnedVideoFor(tab)
	if !ok || ranked == nil {
		return
	}
	for i := range *ranked {
		if (*ranked)[i].VideoID == pinID {
			(*ranked)[i].Score = maxIntScore
			(*ranked)[i].PickReason = "pinned source"
			sortAudioSources(*ranked)
			return
		}
	}
	*ranked = append([]AudioSource{{
		ID:         "yt:" + pinID,
		Kind:       SourceOnline,
		Label:      "pinned video",
		VideoID:    pinID,
		Score:      maxIntScore,
		Detail:     "pinned source · online",
		PickReason: "pinned source",
	}}, (*ranked)...)
}

// ResolveAudio finds a local backing track or downloads the best online match.
func ResolveAudio(tab *model.Tab, tabPath string, extraDirs []string, allowOnline bool) (string, error) {
	if path := FindAudio(tab, tabPath, extraDirs); path != "" {
		return path, nil
	}
	if !allowOnline {
		return "", nil
	}
	src, err := BestOnlineSource(tab)
	if err != nil {
		return "", err
	}
	if src == nil {
		return "", nil
	}
	return EnsureAudioSource(tab, *src)
}

// BestOnlineSource returns the top-ranked online candidate.
func BestOnlineSource(tab *model.Tab) (*AudioSource, error) {
	// A pinned video wins without a search: the user's explicit choice is
	// the answer, and it must hold even when the search engine is down.
	if pinID, ok := PinnedVideoFor(tab); ok {
		return &AudioSource{
			ID:         "yt:" + pinID,
			Kind:       SourceOnline,
			Label:      "pinned video",
			VideoID:    pinID,
			Score:      maxIntScore,
			Detail:     "pinned source · online",
			PickReason: "pinned source",
		}, nil
	}
	cands, err := SearchOnlineCandidates(tab, 5)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, errors.New("no online audio matches found")
	}
	src := cands[0]
	return &src, nil
}

// EnsureAudioSource returns a playable file path for a source, downloading if needed.
func EnsureAudioSource(tab *model.Tab, src AudioSource) (string, error) {
	switch src.Kind {
	case SourceMIDI:
		return "", nil
	case SourceLocal:
		if src.Path != "" && fileExists(src.Path) {
			return src.Path, nil
		}
		return "", fmt.Errorf("local audio not found: %s", src.Label)
	case SourceOnline:
		if src.Path != "" && fileExists(src.Path) {
			return src.Path, nil
		}
		if src.VideoID == "" {
			return "", errors.New("missing video id for online audio")
		}
		return DownloadYouTubeAudio(tab, src.VideoID, src.Duration)
	default:
		return "", fmt.Errorf("unknown audio source kind: %s", src.Kind)
	}
}

// DownloadYouTubeAudio fetches audio for a specific YouTube video id,
// validating the result against the search entry's duration (within 30%) so
// a mismatched download is caught and removed instead of silently becoming
// "the" audio.
func DownloadYouTubeAudio(tab *model.Tab, videoID string, expected time.Duration) (string, error) {
	if videoID == "" {
		return "", errors.New("empty video id")
	}
	if !OnlineAudioAvailable() {
		return "", errYtDlpMissing
	}
	if path := cachedPathForVideo(tab, videoID); fileExists(path) {
		return path, nil
	}

	dir, err := config.AudioDir()
	if err != nil {
		return "", err
	}
	targetBase := filepath.Join(dir, sanitizeAudioFilename(cacheAudioBasename(tab))+" ["+videoID+"]")
	outTemplate := targetBase + ".%(ext)s"
	url := "https://www.youtube.com/watch?v=" + videoID
	args := []string{
		url,
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "5",
		"-o", outTemplate,
		"--no-playlist",
		"--no-warnings",
		"--quiet",
		"--newline",
	}
	ctx, cancel := context.WithTimeout(context.Background(), ytDownloadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("download audio: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("download audio: %w", err)
	}
	// Feed yt-dlp's --newline progress lines to the UI hook while the
	// download runs; ParseProgressLine ignores non-progress lines.
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		NotifyDownloadProgress(scanner.Text())
	}
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("download audio: %w", err)
	}
	for _, ext := range audioExtensions {
		p := targetBase + ext
		if fileExists(p) {
			// Post-download validation: the file must be plausibly the same
			// recording the search promised.
			if expected > 0 {
				if dur, err := ProbeDuration(p); err == nil && dur > 0 {
					diff := float64(dur-expected) / float64(expected)
					if diff < -0.3 || diff > 0.3 {
						_ = os.Remove(p)
						return "", fmt.Errorf("downloaded audio does not match the search result (%s vs expected %s) — try another source", formatDuration(dur), formatDuration(expected))
					}
				}
			}
			// New cache entry: enforce the configured cap (LRU eviction).
			_ = EnforceCacheCap()
			return p, nil
		}
	}
	return "", errors.New("download finished but audio file was not found")
}

// FetchAudioOnline downloads the best-ranked online match (legacy helper).
func FetchAudioOnline(tab *model.Tab) (string, error) {
	src, err := BestOnlineSource(tab)
	if err != nil {
		return "", err
	}
	return EnsureAudioSource(tab, *src)
}
