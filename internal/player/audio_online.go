package player

import (
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

	seen := map[string]struct{}{}
	var ranked []AudioSource
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
			score := ScoreYouTubeResult(tab, e.Title, channel, e.Description, e.Duration)
			dur := time.Duration(e.Duration) * time.Second
			path := cachedPathForVideo(tab, e.ID)
			if fileExists(path) {
				if probed, err := ProbeDuration(path); err == nil && probed > 0 {
					dur = probed
				}
			}
			src := AudioSource{
				ID:       "yt:" + e.ID,
				Kind:     SourceOnline,
				Label:    e.Title,
				Path:     path,
				VideoID:  e.ID,
				Duration: dur,
				Score:    score,
				Detail:   formatDuration(dur) + " · " + channel + " · online",
				Category: cat,
				StrictOK: strictOK,
			}
			ranked = append(ranked, src)
		}
	}

	sortAudioSources(ranked)
	ranked = ranked[:min(len(ranked), limit*2)]
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
				ranked = append(ranked, AudioSource{
					ID:       "yt:" + e.ID,
					Kind:     SourceOnline,
					Label:    e.Title,
					Path:     cachedPathForVideo(tab, e.ID),
					VideoID:  e.ID,
					Duration: time.Duration(e.Duration) * time.Second,
					Score:    ScoreYouTubeResult(tab, e.Title, channel, e.Description, e.Duration),
					Detail:   formatDuration(time.Duration(e.Duration)*time.Second) + " · " + channel + " · online",
					Category: cat,
					StrictOK: strictOK,
				})
			}
		}
		sortAudioSources(ranked)
	}
	// A total failure must not be reported as "no matches": the real cause
	// (yt-dlp missing, timed out, network error) is what the user needs.
	if len(ranked) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return ranked, nil
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
		"--no-progress",
	}
	ctx, cancel := context.WithTimeout(context.Background(), ytDownloadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	if err := cmd.Run(); err != nil {
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
