package scraper

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
)

// ugHTMLClient fetches tabs by scraping Ultimate Guitar web pages.
type ugHTMLClient struct {
	http    *http.Client
	delay   time.Duration
	lastReq time.Time
}

func newUGHTMLClient(delay time.Duration) *ugHTMLClient {
	return &ugHTMLClient{
		http:  &http.Client{Timeout: 30 * time.Second},
		delay: delay,
	}
}

func (c *ugHTMLClient) sleep() {
	if c.delay <= 0 {
		return
	}
	since := time.Since(c.lastReq)
	if since < c.delay {
		time.Sleep(c.delay - since)
	}
	c.lastReq = time.Now()
}

func (c *ugHTMLClient) Search(query string) ([]SearchResult, error) {
	c.sleep()
	u := "https://www.ultimate-guitar.com/search.php?search_type=title&value=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ugBrowserUA)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ug html search: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	payload, err := extractUGDataContent(body)
	if err != nil {
		return nil, err
	}
	var page struct {
		Store struct {
			Page struct {
				Data struct {
					Results []struct {
						ID            int64   `json:"id"`
						TabID         int64   `json:"tab_id"`
						SongName      string  `json:"song_name"`
						ArtistName    string  `json:"artist_name"`
						Type          string  `json:"type"`
						Rating        float64 `json:"rating"`
						Votes         int64   `json:"votes"`
						TabAccessType string  `json:"tab_access_type"`
					} `json:"results"`
				} `json:"data"`
			} `json:"page"`
		} `json:"store"`
	}
	if err := json.Unmarshal(payload, &page); err != nil {
		return nil, fmt.Errorf("ug html search decode: %w", err)
	}
	var out []SearchResult
	for _, t := range page.Store.Page.Data.Results {
		id := t.ID
		if id == 0 {
			id = t.TabID
		}
		if id == 0 {
			continue
		}
		if t.Type != "" && !strings.EqualFold(t.Type, "Tabs") && !strings.EqualFold(t.Type, "Chords") {
			continue
		}
		out = append(out, SearchResult{
			ID:         id,
			Source:     SourceUG,
			SongName:   t.SongName,
			ArtistName: t.ArtistName,
			Type:       t.Type,
			Rating:     t.Rating,
			Votes:      t.Votes,
		})
	}
	return out, nil
}

func (c *ugHTMLClient) Fetch(id int64) (*model.Tab, error) {
	c.sleep()
	tabURL := fmt.Sprintf("https://tabs.ultimate-guitar.com/tab/_/_tabs_%d", id)
	req, err := http.NewRequest(http.MethodGet, tabURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ugBrowserUA)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ug html fetch %d: %w", id, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	payload, err := extractUGDataContent(body)
	if err != nil {
		return nil, err
	}
	var page struct {
		Store struct {
			Page struct {
				Data struct {
					TabView struct {
						WikiTab struct {
							Content string `json:"content"`
						} `json:"wiki_tab"`
					} `json:"tab_view"`
					Tab struct {
						SongName   string `json:"song_name"`
						ArtistName string `json:"artist_name"`
						Tuning     string `json:"tuning"`
						Capo       int    `json:"capo"`
					} `json:"tab"`
				} `json:"data"`
			} `json:"page"`
		} `json:"store"`
	}
	if err := json.Unmarshal(payload, &page); err != nil {
		return nil, fmt.Errorf("ug html fetch decode: %w", err)
	}
	content := page.Store.Page.Data.TabView.WikiTab.Content
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("ug html fetch %d: empty tab content", id)
	}
	content = normalizeContent(content)
	tab, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse fetched tab: %w", err)
	}
	if tab.Title == "" {
		tab.Title = page.Store.Page.Data.Tab.SongName
	}
	if tab.Artist == "" {
		tab.Artist = page.Store.Page.Data.Tab.ArtistName
	}
	if len(tab.Tuning) == 0 && page.Store.Page.Data.Tab.Tuning != "" {
		tab.Tuning = model.ParseTuning(strings.ReplaceAll(page.Store.Page.Data.Tab.Tuning, " ", ""))
	}
	if page.Store.Page.Data.Tab.Capo > 0 {
		if tab.Metadata == nil {
			tab.Metadata = map[string]string{}
		}
		tab.Metadata["capo"] = strconv.Itoa(page.Store.Page.Data.Tab.Capo)
	}
	player.NormalizeTabBPM(tab)
	return tab, nil
}

const ugBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func extractUGDataContent(body []byte) ([]byte, error) {
	s := string(body)
	const marker = `data-content="`
	idx := strings.Index(s, marker)
	if idx < 0 {
		return nil, fmt.Errorf("ug html: data-content not found")
	}
	start := idx + len(marker)
	end := start
	for end < len(s) && s[end] != '"' {
		end++
	}
	raw := html.UnescapeString(s[start:end])
	return []byte(raw), nil
}
