package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/config"
	"github.com/YOUR_USERNAME/fretboard/internal/model"
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

// AudioSearchQueries returns several search phrases tuned for guitar tabs.
func AudioSearchQueries(tab *model.Tab) []string {
	if tab == nil {
		return nil
	}
	artist := strings.TrimSpace(tab.Artist)
	title := strings.TrimSpace(tab.Title)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if strings.EqualFold(existing, s) {
				return
			}
		}
		out = append(out, s)
	}
	if artist != "" && title != "" {
		add(artist + " " + title + " guitar")
		add(artist + " " + title + " official audio")
		add(artist + " " + title + " studio")
		add(artist + " " + title)
	} else if title != "" {
		add(title + " guitar")
		add(title)
	} else if artist != "" {
		add(artist)
	}
	return out
}

var (
	ytSearchTimeout   = 12 * time.Second
	ytDownloadTimeout = 10 * time.Minute
)

type ytSearchEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Channel     string `json:"channel"`
	Uploader    string `json:"uploader"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
}

type ytSearchPlaylist struct {
	Entries []ytSearchEntry `json:"entries"`
}

// SearchOnlineCandidates queries YouTube and ranks matches for the tab.
func SearchOnlineCandidates(tab *model.Tab, limit int) ([]AudioSource, error) {
	if tab == nil {
		return nil, errors.New("nil tab")
	}
	if !OnlineAudioAvailable() {
		return nil, errors.New("yt-dlp not found — install: sudo pacman -S yt-dlp")
	}
	if limit <= 0 {
		limit = 5
	}

	seen := map[string]struct{}{}
	var ranked []AudioSource

	for _, query := range AudioSearchQueries(tab) {
		entries, err := ytSearch(query, limit)
		if err != nil {
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
			}
			ranked = append(ranked, src)
		}
	}

	sortAudioSources(ranked)
	if len(ranked) > limit*2 {
		ranked = ranked[:limit*2]
	}
	return ranked, nil
}

func ytSearch(query string, limit int) ([]ytSearchEntry, error) {
	args := []string{
		"--dump-single-json",
		"--flat-playlist",
		"--no-warnings",
		"--quiet",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	}
	ctx, cancel := context.WithTimeout(context.Background(), ytSearchTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", args...).Output()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("yt-dlp search timed out")
		}
		return nil, err
	}
	var playlist ytSearchPlaylist
	if err := json.Unmarshal(out, &playlist); err != nil {
		return nil, err
	}
	return playlist.Entries, nil
}

func sortAudioSources(sources []AudioSource) {
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[j].Score > sources[i].Score {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}
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
		return DownloadYouTubeAudio(tab, src.VideoID)
	default:
		return "", fmt.Errorf("unknown audio source kind: %s", src.Kind)
	}
}

// DownloadYouTubeAudio fetches audio for a specific YouTube video id.
func DownloadYouTubeAudio(tab *model.Tab, videoID string) (string, error) {
	if videoID == "" {
		return "", errors.New("empty video id")
	}
	if !OnlineAudioAvailable() {
		return "", errors.New("yt-dlp not found — install: sudo pacman -S yt-dlp")
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

func cacheAudioBasename(tab *model.Tab) string {
	names := audioNameCandidates(tab)
	if len(names) > 1 {
		return names[1]
	}
	if len(names) > 0 {
		return names[0]
	}
	return "track"
}

func cachedPathForVideo(tab *model.Tab, videoID string) string {
	dir, err := config.AudioDir()
	if err != nil {
		return ""
	}
	base := sanitizeAudioFilename(cacheAudioBasename(tab)) + " [" + videoID + "]"
	for _, ext := range audioExtensions {
		p := filepath.Join(dir, base+ext)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func findCachedOnlineAudio(tab *model.Tab) string {
	dir, err := config.AudioDir()
	if err != nil {
		return ""
	}
	return findAudioByNames(dir, audioNameCandidates(tab))
}

func sanitizeAudioFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-",
		"*", "", "?", "", "\"", "",
		"<", "", ">", "", "|", "",
	)
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "track"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}
