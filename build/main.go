package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port               string
	IconSource         string
	JSDelivrURL        string
	LocalPath          string
	StandardIconFormat string
	CacheTTL           time.Duration
	CacheSize          int
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
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return "", false
	}

	if time.Since(item.Timestamp) > c.ttl {
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
	config *Config
	cache  *Cache
)

func loadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4050"
	}

	iconSource := os.Getenv("ICON_SOURCE")
	if iconSource == "" {
		iconSource = "remote"
	}

	standardFormat := os.Getenv("STANDARD_ICON_FORMAT")
	if standardFormat == "" {
		standardFormat = "svg"
	}

	if standardFormat != "svg" && standardFormat != "png" && standardFormat != "webp" && standardFormat != "avif" && standardFormat != "ico" {
		standardFormat = "svg"
	}

	return &Config{
		Port:               port,
		IconSource:         iconSource,
		JSDelivrURL:        "https://cdn.jsdelivr.net/gh/selfhst/icons@main",
		LocalPath:          "/app/icons",
		StandardIconFormat: standardFormat,
		CacheTTL:           time.Hour,
		CacheSize:          500,
	}
}

func isValidHexColor(color string) bool {
	matched, _ := regexp.MatchString("^[0-9A-Fa-f]{6}$", color)
	return matched
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func urlExists(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func readLocalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func fetchRemoteFile(url string) (string, error) {
	resp, err := http.Get(url)
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
	re1 := regexp.MustCompile(`style="[^"]*fill:\s*#fff[^"]*"`)
	svgContent = re1.ReplaceAllStringFunc(svgContent, func(match string) string {
		re2 := regexp.MustCompile(`fill:\s*#fff`)
		return re2.ReplaceAllString(match, "fill:"+color)
	})

	re3 := regexp.MustCompile(`fill="#fff"`)
	svgContent = re3.ReplaceAllString(svgContent, `fill="`+color+`"`)

	// Replace stop-color:#fff in gradients
	re4 := regexp.MustCompile(`style="[^"]*stop-color:\s*#fff[^"]*"`)
	svgContent = re4.ReplaceAllStringFunc(svgContent, func(match string) string {
		re5 := regexp.MustCompile(`stop-color:\s*#fff`)
		return re5.ReplaceAllString(match, "stop-color:"+color)
	})

	re6 := regexp.MustCompile(`stop-color="#fff"`)
	svgContent = re6.ReplaceAllString(svgContent, `stop-color="`+color+`"`)

	return svgContent
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
	default:
		return "image/svg+xml"
	}
}

func getCacheKey(iconName, colorCode string) string {
	if colorCode == "" {
		return iconName + ":default"
	}
	return iconName + ":" + colorCode
}

func handleIcon(w http.ResponseWriter, r *http.Request) {
	iconName := r.PathValue("iconname")
	colorCode := r.PathValue("colorcode")

	if iconName == "" {
		http.Error(w, "Icon name is required", http.StatusBadRequest)
		return
	}

	if colorCode != "" && !isValidHexColor(colorCode) {
		log.Printf("[ERROR] Invalid color code for icon \"%s\": %s", iconName, colorCode)
		http.Error(w, "Invalid color code. Use 6-digit hex without #", http.StatusBadRequest)
		return
	}

	cacheKey := getCacheKey(iconName, colorCode)

	var contentType string
	var formatToServe string
	
	if colorCode != "" {
		contentType = "image/svg+xml"
		formatToServe = "svg"
	} else {
		contentType = getContentType(config.StandardIconFormat)
		formatToServe = config.StandardIconFormat
	}

	if cached, found := cache.Get(cacheKey); found {
		log.Printf("[CACHE] Serving cached icon: \"%s\"%s (%s)", iconName, 
			func() string { if colorCode != "" { return " with color " + colorCode } else { return "" } }(),
			formatToServe)
		w.Header().Set("Content-Type", contentType)
		w.Write([]byte(cached))
		return
	}

	var iconContent string
	var err error

	if config.IconSource == "local" {
		if colorCode != "" {
			lightPath := filepath.Join(config.LocalPath, "svg", iconName+"-light.svg")
			if fileExists(lightPath) {
				iconContent, err = readLocalFile(lightPath)
				if err == nil {
					iconContent = applySVGColor(iconContent, colorCode)
				}
			}
		} else {
			var standardPath string
			if formatToServe == "svg" {
				standardPath = filepath.Join(config.LocalPath, "svg", iconName+".svg")
			} else {
				standardPath = filepath.Join(config.LocalPath, formatToServe, iconName+"."+formatToServe)
			}
			
			if fileExists(standardPath) {
				iconContent, err = readLocalFile(standardPath)
			}
		}
		
		if iconContent == "" {
			svgPath := filepath.Join(config.LocalPath, "svg", iconName+".svg")
			if fileExists(svgPath) {
				iconContent, err = readLocalFile(svgPath)
				contentType = "image/svg+xml"
				formatToServe = "svg"
			}
		}
	} else {
		if colorCode != "" {
			lightURL := config.JSDelivrURL + "/svg/" + iconName + "-light.svg"
			if urlExists(lightURL) {
				iconContent, err = fetchRemoteFile(lightURL)
				if err == nil {
					iconContent = applySVGColor(iconContent, colorCode)
				}
			}
		} else {
			var standardURL string
			if formatToServe == "svg" {
				standardURL = config.JSDelivrURL + "/svg/" + iconName + ".svg"
			} else {
				standardURL = config.JSDelivrURL + "/" + formatToServe + "/" + iconName + "." + formatToServe
			}
			
			if urlExists(standardURL) {
				iconContent, err = fetchRemoteFile(standardURL)
			}
		}
		
		if iconContent == "" {
			svgURL := config.JSDelivrURL + "/svg/" + iconName + ".svg"
			iconContent, err = fetchRemoteFile(svgURL)
			contentType = "image/svg+xml"
			formatToServe = "svg"
		}
	}

	if iconContent == "" || err != nil {
		log.Printf("[ERROR] Icon not found: \"%s\"%s (source: %s)", iconName,
			func() string { if colorCode != "" { return " with color " + colorCode } else { return "" } }(),
			config.IconSource)
		http.Error(w, "Icon not found", http.StatusNotFound)
		return
	}

	cache.Set(cacheKey, iconContent)

	log.Printf("[SUCCESS] Serving icon: \"%s\"%s (%s, source: %s)", iconName,
		func() string { if colorCode != "" { return " with color " + colorCode } else { return "" } }(),
		formatToServe, config.IconSource)

	w.Header().Set("Content-Type", contentType)
	w.Write([]byte(iconContent))
}

func handleCustomIcon(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")

	if filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	customPath := filepath.Join("/app/icons/custom", filename)

	log.Printf("[DEBUG] Looking for custom icon at: %s", customPath)

	if !fileExists(customPath) {
		if files, err := os.ReadDir("/app/icons/custom"); err == nil {
			var fileList []string
			for _, file := range files {
				fileList = append(fileList, file.Name())
			}
			log.Printf("[DEBUG] Files in /app/icons/custom: %v", fileList)
		} else {
			log.Printf("[DEBUG] Failed to read /app/icons/custom directory: %v", err)
		}
		log.Printf("[ERROR] Custom icon not found: \"%s\" at path: %s", filename, customPath)
		http.Error(w, "Custom icon not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(customPath)
	if err != nil {
		log.Printf("[ERROR] Failed to read custom icon \"%s\": %v", filename, err)
		http.Error(w, "Failed to read custom icon", http.StatusInternalServerError)
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	var contentType string
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".svg":
		contentType = "image/svg+xml"
	case ".webp":
		contentType = "image/webp"
	case ".avif":
		contentType = "image/avif"
	case ".ico":
		contentType = "image/x-icon"
	default:
		contentType = "application/octet-stream"
	}

	log.Printf("[SUCCESS] Serving custom icon: \"%s\" (%s)", filename, contentType)

	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	configInfo := map[string]interface{}{
		"server":    "Self-hosted icon server",
		"urlFormat": "https://subdomain.example.com/iconname/colorcode",
		"features": map[string]interface{}{
			"iconSource": func() string {
				if config.IconSource == "local" {
					return "Local volume"
				}
				return "Remote CDN"
			}(),
			"standardFormat": config.StandardIconFormat,
			"caching": fmt.Sprintf("TTL: %ds, Max items: %d", int(config.CacheTTL.Seconds()), config.CacheSize),
			"baseUrl": func() string {
				if config.IconSource == "local" {
					return config.LocalPath
				}
				return config.JSDelivrURL
			}(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configInfo)
}

func main() {
	config = loadConfig()
	cache = NewCache(config.CacheTTL, config.CacheSize)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /custom/{filename}", handleCustomIcon)

	// Suppress favicon load error message in logs when viewing via browser
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /{iconname}/{colorcode}", handleIcon)
	mux.HandleFunc("GET /{iconname}", handleIcon)

	mux.HandleFunc("GET /", handleRoot)

	log.Printf("Icon server listening on port %s", config.Port)
	log.Printf("Icon source: %s", func() string {
		if config.IconSource == "local" {
			return "Local volume"
		}
		return "Remote CDN"
	}())
	log.Printf("Cache settings: TTL %ds, Max %d items", int(config.CacheTTL.Seconds()), config.CacheSize)

	log.Fatal(http.ListenAndServe(":"+config.Port, mux))
}