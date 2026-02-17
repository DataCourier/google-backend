package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"focus-tube/auth"
	"focus-tube/buckets"
)

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// Don't override env vars already set
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadEnvFile(".env")
	ctx := context.Background()

	if os.Getenv("ENV") != "production" && os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		log.Fatal("FIRESTORE_EMULATOR_HOST not set. Run: gcloud emulators firestore start --host-port=localhost:9090")
	}

	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = "michal-playground-2026"
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	authService := auth.NewAuthService(client)
	authMw := auth.CombinedMiddleware(authService)

	// Auth routes (unauthenticated)
	auth.RegisterRoutes(r, authService)

	// Bucket routes
	buckets.RegisterOpenBucketRoutes(r, client, authMw)

	// API routes
	r.Group(func(r chi.Router) {
		r.Use(authMw)
		r.Get("/api/resolve-channel", resolveChannelHandler)
		r.Get("/api/fetch-rss", fetchRSSHandler)
		r.Get("/api/fetch-all-videos", fetchAllVideosHandler)
	})

	// Static files — serve frontend/dist
	frontendDir := "frontend/dist"
	if _, err := os.Stat(frontendDir); err == nil {
		fileServer := http.FileServer(http.Dir(frontendDir))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try serving the file; fall back to index.html for SPA routing
			path := frontendDir + r.URL.Path
			if _, err := os.Stat(path); os.IsNotExist(err) {
				http.ServeFile(w, r, frontendDir+"/index.html")
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("FocusTube starting on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

// resolveChannelHandler fetches a YouTube channel page and extracts channel ID/name.
// GET /api/resolve-channel?url=https://youtube.com/@SomeChannel
func resolveChannelHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "url parameter required"})
		return
	}

	// Normalize URL
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	req2, _ := http.NewRequest("GET", url, nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req2.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := http.DefaultClient.Do(req2)
	if err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch URL: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read response"})
		return
	}
	html := string(body)

	// Extract channel ID from canonical link or meta tags
	channelID := ""
	channelName := ""

	// Try: canonical link (handles both attribute orders)
	canonicalRe := regexp.MustCompile(`youtube\.com/channel/(UC[a-zA-Z0-9_-]+)`)
	if m := canonicalRe.FindStringSubmatch(html); len(m) > 1 {
		channelID = m[1]
	}

	// Try: "channelId":"UCxxxxxx" or "channelId": "UCxxxxxx"
	if channelID == "" {
		jsonRe := regexp.MustCompile(`"channelId"\s*:\s*"(UC[a-zA-Z0-9_-]+)"`)
		if m := jsonRe.FindStringSubmatch(html); len(m) > 1 {
			channelID = m[1]
		}
	}

	// Try: <meta itemprop="channelId" content="UCxxxxxx">
	if channelID == "" {
		metaRe := regexp.MustCompile(`itemprop="channelId"\s+content="(UC[a-zA-Z0-9_-]+)"`)
		if m := metaRe.FindStringSubmatch(html); len(m) > 1 {
			channelID = m[1]
		}
	}

	// Broadest fallback: any UC channel ID pattern
	if channelID == "" {
		ucRe := regexp.MustCompile(`UC[a-zA-Z0-9_-]{20,}`)
		if m := ucRe.FindString(html); m != "" {
			channelID = m
		}
	}

	if channelID == "" {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "could not find channel ID in page"})
		return
	}

	// Extract channel name from <title> or og:title
	titleRe := regexp.MustCompile(`<title>([^<]+)</title>`)
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		channelName = strings.TrimSuffix(m[1], " - YouTube")
		channelName = strings.TrimSpace(channelName)
	}

	rssURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id":   channelID,
		"channel_name": channelName,
		"rss_url":      rssURL,
	})
}

// Atom feed structures
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Group struct {
		Thumbnail struct {
			URL string `xml:"url,attr"`
		} `xml:"http://search.yahoo.com/mrss/ thumbnail"`
		Description string `xml:"http://search.yahoo.com/mrss/ description"`
	} `xml:"http://search.yahoo.com/mrss/ group"`
}

// fetchRSSHandler fetches and parses a YouTube channel's Atom RSS feed.
// GET /api/fetch-rss?channel_id=UCxxxxxx
func fetchRSSHandler(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id parameter required"})
		return
	}

	feedURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	resp, err := http.Get(feedURL)
	if err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch RSS: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read RSS"})
		return
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to parse RSS: " + err.Error()})
		return
	}

	videos := make([]map[string]interface{}, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		videos = append(videos, map[string]interface{}{
			"video_id":    e.VideoID,
			"title":       e.Title,
			"published":   e.Published,
			"channel":     e.Author.Name,
			"thumbnail":   e.Group.Thumbnail.URL,
			"description": e.Group.Description,
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"channel":    feed.Title,
		"channel_id": channelID,
		"videos":     videos,
	})
}

// fetchAllVideosHandler uses YouTube Data API v3 to fetch all videos from a channel.
// GET /api/fetch-all-videos?channel_id=UCxxxxxx
// Requires YOUTUBE_API_KEY env var.
func fetchAllVideosHandler(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id parameter required"})
		return
	}

	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "YOUTUBE_API_KEY not configured"})
		return
	}

	// Step 1: Get the channel's "uploads" playlist ID (replace UC with UU)
	uploadsPlaylistID := "UU" + channelID[2:]

	// Step 2: Paginate through playlistItems
	var allVideos []map[string]interface{}
	pageToken := ""

	for {
		apiURL := fmt.Sprintf(
			"https://www.googleapis.com/youtube/v3/playlistItems?part=snippet&playlistId=%s&maxResults=50&key=%s",
			uploadsPlaylistID, apiKey,
		)
		if pageToken != "" {
			apiURL += "&pageToken=" + pageToken
		}

		resp, err := http.Get(apiURL)
		if err != nil {
			respondJSON(w, http.StatusBadGateway, map[string]string{"error": "YouTube API request failed: " + err.Error()})
			return
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			respondJSON(w, http.StatusBadGateway, map[string]string{"error": "YouTube API error: " + string(body)})
			return
		}

		var result struct {
			NextPageToken string `json:"nextPageToken"`
			Items         []struct {
				Snippet struct {
					Title        string `json:"title"`
					Description  string `json:"description"`
					PublishedAt  string `json:"publishedAt"`
					ChannelTitle string `json:"channelTitle"`
					Thumbnails   struct {
						Medium struct {
							URL string `json:"url"`
						} `json:"medium"`
					} `json:"thumbnails"`
					ResourceID struct {
						VideoID string `json:"videoId"`
					} `json:"resourceId"`
				} `json:"snippet"`
			} `json:"items"`
			PageInfo struct {
				TotalResults int `json:"totalResults"`
			} `json:"pageInfo"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			respondJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to parse YouTube API response"})
			return
		}

		for _, item := range result.Items {
			s := item.Snippet
			allVideos = append(allVideos, map[string]interface{}{
				"video_id":    s.ResourceID.VideoID,
				"title":       s.Title,
				"published":   s.PublishedAt,
				"channel":     s.ChannelTitle,
				"thumbnail":   s.Thumbnails.Medium.URL,
				"description": s.Description,
			})
		}

		log.Printf("Fetched %d videos so far for %s (total: %d)", len(allVideos), channelID, result.PageInfo.TotalResults)

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id": channelID,
		"videos":     allVideos,
		"total":      len(allVideos),
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := os.Getenv("CORS_ORIGINS") // comma-separated
		if allowed == "" {
			allowed = "http://localhost:5173"
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
