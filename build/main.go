package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	logLevelDebug = 0
	logLevelInfo  = 1
	logLevelError = 2
)

type Config struct {
	Port          string
	IconSource    string
	RemoteURL     string
	LocalPath     string
	PrimaryColor  string
	CacheTTL      time.Duration
	CacheSize     int
	RemoteTimeout time.Duration
	CORSOrigins   []string
	LogLevel      int
}

type CacheItem struct {
	Content   string
	Timestamp time.Time
}

type Cache struct {
	items map[string]CacheItem
	mutex sync.RWMutex
	ttl   time.Duration
	max   int
}

func NewCache(ttl time.Duration, maxSize int) *Cache {
	return &Cache{
		items: make(map[string]CacheItem),
		ttl:   ttl,
		max:   maxSize,
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		return "", false
	}

	if time.Since(item.Timestamp) > c.ttl {
		c.mutex.Lock()
		if current, exists := c.items[key]; exists && time.Since(current.Timestamp) > c.ttl {
			delete(c.items, key)
		}
		c.mutex.Unlock()
		return "", false
	}

	return item.Content, true
}

func (c *Cache) Set(key, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.items) >= c.max {
		var oldestKey string
		var oldestTime time.Time
		first := true

		for k, v := range c.items {
			if first || v.Timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.Timestamp
				first = false
			}
		}
		delete(c.items, oldestKey)
	}

	c.items[key] = CacheItem{
		Content:   value,
		Timestamp: time.Now(),
	}
}

var (
	config     *Config
	cache      *Cache
	httpClient *http.Client
)

var (
	hexColorRe  = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)
	reFillStyle = regexp.MustCompile(`style="[^"]*fill:\s*#fff(?:fff)?[^"]*"`)
	reFillInner = regexp.MustCompile(`fill:\s*#fff(?:fff)?`)
	reFillAttr  = regexp.MustCompile(`fill="#fff(?:fff)?"`)
	reStopStyle = regexp.MustCompile(`style="[^"]*stop-color:\s*#fff(?:fff)?[^"]*"`)
	reStopInner = regexp.MustCompile(`stop-color:\s*#fff(?:fff)?`)
	reStopAttr  = regexp.MustCompile(`stop-color="#fff(?:fff)?"`)
)

func logf(level int, format string, args ...any) {
	if level >= config.LogLevel {
		log.Printf(format, args...)
	}
}

func formatDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
}

func computeETag(content string) string {
	h := fnv.New64a()
	h.Write([]byte(content))
	return fmt.Sprintf(`"%x"`, h.Sum64())
}

func loadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4050"
	}

	iconSource := os.Getenv("ICON_SOURCE")
	if iconSource == "" {
		iconSource = "remote"
	}

	remoteURL := os.Getenv("REMOTE_URL")
	if remoteURL == "" {
		remoteURL = "https://cdn.jsdelivr.net/gh/selfhst/icons@main"
	}

	primaryColor := strings.TrimPrefix(os.Getenv("PRIMARY_COLOR"), "#")

	cacheTTL := time.Hour
	if v := os.Getenv("CACHE_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cacheTTL = time.Duration(n) * time.Second
		} else {
			log.Printf("[WARN] Invalid CACHE_TTL value \"%s\", using default (3600)", v)
		}
	}

	cacheSize := 500
	if v := os.Getenv("CACHE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cacheSize = n
		} else {
			log.Printf("[WARN] Invalid CACHE_SIZE value \"%s\", using default (500)", v)
		}
	}

	remoteTimeout := 10 * time.Second
	if v := os.Getenv("REMOTE_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			remoteTimeout = time.Duration(n) * time.Second
		} else {
			log.Printf("[WARN] Invalid REMOTE_TIMEOUT value \"%s\", using default (10)", v)
		}
	}

	corsAllowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if corsAllowedOrigins == "" {
		corsAllowedOrigins = "*"
	}
	var corsOrigins []string
	for _, o := range strings.Split(corsAllowedOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			corsOrigins = append(corsOrigins, trimmed)
		}
	}

	logLevel := logLevelInfo
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			logLevel = logLevelDebug
		case "info":
			logLevel = logLevelInfo
		case "error":
			logLevel = logLevelError
		default:
			log.Printf("[WARN] Invalid LOG_LEVEL value \"%s\", using default (info)", v)
		}
	}

	return &Config{
		Port:          port,
		IconSource:    iconSource,
		RemoteURL:     remoteURL,
		LocalPath:     "/app/icons",
		PrimaryColor:  primaryColor,
		CacheTTL:      cacheTTL,
		CacheSize:     cacheSize,
		RemoteTimeout: remoteTimeout,
		CORSOrigins:   corsOrigins,
		LogLevel:      logLevel,
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(config.CORSOrigins) == 1 && config.CORSOrigins[0] == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			origin := r.Header.Get("Origin")
			for _, allowed := range config.CORSOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
					break
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func validateConfig(cfg *Config) {
	if cfg.IconSource != "local" && cfg.IconSource != "remote" && cfg.IconSource != "hybrid" {
		log.Fatalf("[ERROR] Invalid ICON_SOURCE \"%s\": must be \"local\", \"remote\", or \"hybrid\"", cfg.IconSource)
	}
	if cfg.IconSource == "local" || cfg.IconSource == "hybrid" {
		info, err := os.Stat(cfg.LocalPath)
		if err != nil {
			log.Fatalf("[ERROR] Icon path \"%s\" is not accessible: %v", cfg.LocalPath, err)
		}
		if !info.IsDir() {
			log.Fatalf("[ERROR] Icon path \"%s\" is not a directory", cfg.LocalPath)
		}
	}
	if cfg.PrimaryColor != "" && !isValidHexColor(cfg.PrimaryColor) {
		log.Fatalf("[ERROR] PRIMARY_COLOR \"%s\" is not a valid 6-digit hex color", cfg.PrimaryColor)
	}
	for _, origin := range cfg.CORSOrigins {
		if origin != "*" && !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			log.Printf("[WARN] CORS_ALLOWED_ORIGINS entry \"%s\" is missing a scheme — did you mean \"https://%s\"?", origin, origin)
		}
	}
}

func isValidHexColor(color string) bool {
	return hexColorRe.MatchString(color)
}

func parseIconName(iconName string) (string, string) {
	ext := strings.ToLower(filepath.Ext(iconName))
	if ext != "" {
		format := strings.TrimPrefix(ext, ".")
		switch format {
		case "svg", "png", "webp", "avif", "ico":
			return strings.TrimSuffix(iconName, filepath.Ext(iconName)), format
		default:
			logf(logLevelError, "[WARN] Unrecognized extension \"%s\" in icon name \"%s\", serving webp instead", ext, iconName)
			return strings.TrimSuffix(iconName, filepath.Ext(iconName)), "webp"
		}
	}
	return iconName, "webp"
}

func readLocalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func fetchRemoteFile(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func applySVGColor(svgContent, colorCode string) string {
	color := "#" + colorCode

	// Replace fill:#fff
	svgContent = reFillStyle.ReplaceAllStringFunc(svgContent, func(match string) string {
		return reFillInner.ReplaceAllString(match, "fill:"+color)
	})
	svgContent = reFillAttr.ReplaceAllString(svgContent, `fill="`+color+`"`)

	// Replace stop-color:#fff in gradients
	svgContent = reStopStyle.ReplaceAllStringFunc(svgContent, func(match string) string {
		return reStopInner.ReplaceAllString(match, "stop-color:"+color)
	})
	svgContent = reStopAttr.ReplaceAllString(svgContent, `stop-color="`+color+`"`)

	return svgContent
}

func serveContent(w http.ResponseWriter, r *http.Request, contentType, content string) {
	if contentType == "image/svg+xml" && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err == nil {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			defer gz.Close()
			io.WriteString(gz, content)
			return
		}
	}
	io.WriteString(w, content)
}

func getContentType(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	case "ico":
		return "image/x-icon"
	case "svg":
		return "image/svg+xml"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	default:
		return ""
	}
}

func getCacheKey(iconName, colorCode string) string {
	if colorCode == "" {
		return iconName + ":default"
	}
	return iconName + ":" + colorCode
}

func handleIcon(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	iconName := r.PathValue("iconname")
	colorCode := r.PathValue("colorcode")

	if iconName == "" {
		http.Error(w, "Icon name is required", http.StatusBadRequest)
		return
	}

	baseName, format := parseIconName(iconName)
	baseName = strings.ToLower(baseName)

	if strings.Contains(baseName, "..") || strings.Contains(baseName, "/") || strings.Contains(baseName, "\\") {
		logf(logLevelError, "[ERROR] Invalid icon name, path traversal attempt: \"%s\"", iconName)
		http.Error(w, "Invalid icon name", http.StatusBadRequest)
		return
	}

	if colorCode == "" {
		colorCode = strings.TrimPrefix(r.URL.Query().Get("color"), "#")
	}

	primaryFallback := false
	if colorCode == "primary" {
		if config.PrimaryColor == "" {
			primaryFallback = true
		}
		colorCode = config.PrimaryColor
	}

	if colorCode != "" && !isValidHexColor(colorCode) {
		logf(logLevelError, "[ERROR] Invalid color code for icon \"%s\": %s", baseName, colorCode)
		http.Error(w, "Invalid color code. Use 6-digit hex without #", http.StatusBadRequest)
		return
	}

	var contentType string
	var formatToServe string

	if colorCode != "" {
		contentType = "image/svg+xml"
		formatToServe = "svg"
	} else {
		formatToServe = format
		contentType = getContentType(formatToServe)
	}

	cacheKey := getCacheKey(baseName+"."+formatToServe, colorCode)

	var colorSuffix string
	if colorCode != "" {
		colorSuffix = " with color " + colorCode
	}

	if cached, found := cache.Get(cacheKey); found {
		logf(logLevelDebug, "[CACHE] Serving cached icon: \"%s\"%s (%s) %v", baseName, colorSuffix, formatToServe, formatDuration(time.Since(start)))
		etag := computeETag(cached)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Cache", "HIT")
		serveContent(w, r, contentType, cached)
		return
	}

	var iconContent string
	var servedFrom string

	if config.IconSource == "local" || config.IconSource == "hybrid" {
		if colorCode != "" {
			lightPath := filepath.Join(config.LocalPath, "svg", baseName+"-light.svg")
			if content, err := readLocalFile(lightPath); err == nil {
				iconContent = applySVGColor(content, colorCode)
				servedFrom = "local"
			}
		} else {
			var standardPath string
			if formatToServe == "svg" {
				standardPath = filepath.Join(config.LocalPath, "svg", baseName+".svg")
			} else {
				standardPath = filepath.Join(config.LocalPath, formatToServe, baseName+"."+formatToServe)
			}
			iconContent, _ = readLocalFile(standardPath)

			if iconContent == "" && formatToServe != "webp" {
				webpPath := filepath.Join(config.LocalPath, "webp", baseName+".webp")
				if content, err := readLocalFile(webpPath); err == nil {
					iconContent = content
					contentType = "image/webp"
					formatToServe = "webp"
				}
			}
			if iconContent != "" {
				servedFrom = "local"
			}
		}
	}

	if iconContent == "" && (config.IconSource == "remote" || config.IconSource == "hybrid") {
		if colorCode != "" {
			lightURL := config.RemoteURL + "/svg/" + baseName + "-light.svg"
			if content, err := fetchRemoteFile(lightURL); err == nil {
				iconContent = applySVGColor(content, colorCode)
				servedFrom = "remote"
			}
		} else {
			var standardURL string
			if formatToServe == "svg" {
				standardURL = config.RemoteURL + "/svg/" + baseName + ".svg"
			} else {
				standardURL = config.RemoteURL + "/" + formatToServe + "/" + baseName + "." + formatToServe
			}
			iconContent, _ = fetchRemoteFile(standardURL)

			if iconContent == "" && formatToServe != "webp" {
				webpURL := config.RemoteURL + "/webp/" + baseName + ".webp"
				if content, err := fetchRemoteFile(webpURL); err == nil {
					iconContent = content
					contentType = "image/webp"
					formatToServe = "webp"
				}
			}
			if iconContent != "" {
				servedFrom = "remote"
			}
		}
	}

	if iconContent == "" {
		logf(logLevelError, "[ERROR] Icon not found: \"%s\"%s (source: %s) %v", baseName, colorSuffix, config.IconSource, formatDuration(time.Since(start)))
		http.Error(w, "Icon not found", http.StatusNotFound)
		return
	}

	cacheKey = getCacheKey(baseName+"."+formatToServe, colorCode)
	cache.Set(cacheKey, iconContent)

	level := "SUCCESS"
	detail := colorSuffix
	if primaryFallback {
		level = "WARN"
		detail = " (PRIMARY_COLOR not set, using default format)"
	}
	logf(logLevelInfo, "[%s] Serving icon: \"%s\"%s (%s, source: %s) %v", level, baseName, detail, formatToServe, servedFrom, formatDuration(time.Since(start)))

	etag := computeETag(iconContent)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Cache", "MISS")
	serveContent(w, r, contentType, iconContent)
}

func handleCustomIcon(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	filename := r.PathValue("filename")

	if filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		logf(logLevelError, "[ERROR] Invalid custom icon filename, path traversal attempt: \"%s\"", filename)
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	filename = strings.ToLower(filename)

	customPath := filepath.Join(config.LocalPath, "custom", filename)

	stat, err := os.Stat(customPath)
	if err != nil {
		if os.IsNotExist(err) {
			logf(logLevelError, "[ERROR] Custom icon not found: \"%s\" %v", filename, formatDuration(time.Since(start)))
			http.Error(w, "Custom icon not found", http.StatusNotFound)
		} else {
			logf(logLevelError, "[ERROR] Failed to read custom icon \"%s\": %v (%v)", filename, err, formatDuration(time.Since(start)))
			http.Error(w, "Failed to read custom icon", http.StatusInternalServerError)
		}
		return
	}

	etag := fmt.Sprintf(`"%d-%d"`, stat.ModTime().Unix(), stat.Size())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	contentType := getContentType(strings.TrimPrefix(ext, "."))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	cacheKey := "custom:" + filename + ":" + etag
	if cached, found := cache.Get(cacheKey); found {
		logf(logLevelDebug, "[CACHE] Serving cached custom icon: \"%s\" %v", filename, formatDuration(time.Since(start)))
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Cache", "HIT")
		serveContent(w, r, contentType, cached)
		return
	}

	data, err := os.ReadFile(customPath)
	if err != nil {
		logf(logLevelError, "[ERROR] Failed to read custom icon \"%s\": %v (%v)", filename, err, formatDuration(time.Since(start)))
		http.Error(w, "Failed to read custom icon", http.StatusInternalServerError)
		return
	}

	cache.Set(cacheKey, string(data))

	logf(logLevelInfo, "[SUCCESS] Serving custom icon: \"%s\" (%s) %v", filename, contentType, formatDuration(time.Since(start)))

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Cache", "MISS")
	serveContent(w, r, contentType, string(data))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "4050"
		}
		resp, err := http.Get("http://localhost:" + port + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	config = loadConfig()
	validateConfig(config)
	cache = NewCache(config.CacheTTL, config.CacheSize)
	httpClient = &http.Client{Timeout: config.RemoteTimeout}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /custom/{filename}", handleCustomIcon)

	// Suppress favicon load error message in logs when viewing via browser
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /{iconname}/{colorcode}", handleIcon)
	mux.HandleFunc("GET /{iconname}", handleIcon)

	log.Printf("Icon server listening on port %s", config.Port)
	log.Printf("Icon source: %s", func() string {
		switch config.IconSource {
		case "local":
			return "Local volume"
		case "hybrid":
			return "Hybrid (local with remote fallback: " + config.RemoteURL + ")"
		default:
			return "Remote: " + config.RemoteURL
		}
	}())
	log.Printf("Cache settings: TTL %ds, Max %d items", int(config.CacheTTL.Seconds()), config.CacheSize)
	log.Printf("Log level: %s", []string{"debug", "info", "error"}[config.LogLevel])

	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped. Bye for now!")
}
