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

	"github.com/gorilla/mux"
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
		delete(c.items, key)
		return "", false
	}

	return item.Content, true
}

func (c *Cache) Set(key, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Remove oldest item if cache is full
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
	// Validate format
	if standardFormat != "svg" && standardFormat != "png" && standardFormat != "webp" {
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
	
	// Replace style fill attributes
	re1 := regexp.MustCompile(`style="[^"]*fill:\s*#fff[^"]*"`)
	svgContent = re1.ReplaceAllStringFunc(svgContent, func(match string) string {
		re2 := regexp.MustCompile(`fill:\s*#fff`)
		return re2.ReplaceAllString(match, "fill:"+color)
	})
	
	// Replace direct fill attributes
	re3 := regexp.MustCompile(`fill="#fff"`)
	svgContent = re3.ReplaceAllString(svgContent, `fill="`+color+`"`)
	
	return svgContent
}

func getContentType(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
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
	vars := mux.Vars(r)
	iconName := vars["iconname"]
	colorCode := vars["colorcode"]

	if iconName == "" {
		http.Error(w, "Icon name is required", http.StatusBadRequest)
		return
	}

	// Validate color if provided
	if colorCode != "" && !isValidHexColor(colorCode) {
		log.Printf("[ERROR] Invalid color code for icon \"%s\": %s", iconName, colorCode)
		http.Error(w, "Invalid color code. Use 6-digit hex without #", http.StatusBadRequest)
		return
	}

	cacheKey := getCacheKey(iconName, colorCode)

	// Determine content type and format to serve
	var contentType string
	var formatToServe string
	
	if colorCode != "" {
		// Always use SVG for colorization
		contentType = "image/svg+xml"
		formatToServe = "svg"
	} else {
		// Use standard format when no color specified
		contentType = getContentType(config.StandardIconFormat)
		formatToServe = config.StandardIconFormat
	}

	// Check cache first
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
		// Use local volume
		if colorCode != "" {
			// Try to find -light version for colorization (always SVG)
			lightPath := filepath.Join(config.LocalPath, "svg", iconName+"-light.svg")
			if fileExists(lightPath) {
				iconContent, err = readLocalFile(lightPath)
				if err == nil {
					iconContent = applySVGColor(iconContent, colorCode)
				}
			}
		} else {
			// No color - try to serve standard format
			var standardPath string
			if formatToServe == "svg" {
				standardPath = filepath.Join(config.LocalPath, "svg", iconName+".svg")
			} else {
				// For PNG/WebP, use format-specific directories
				standardPath = filepath.Join(config.LocalPath, formatToServe, iconName+"."+formatToServe)
			}
			
			if fileExists(standardPath) {
				iconContent, err = readLocalFile(standardPath)
			}
		}
		
		// Fall back to SVG if standard format not found
		if iconContent == "" {
			svgPath := filepath.Join(config.LocalPath, "svg", iconName+".svg")
			if fileExists(svgPath) {
				iconContent, err = readLocalFile(svgPath)
				contentType = "image/svg+xml"
				formatToServe = "svg"
			}
		}
	} else {
		// Use remote CDN
		if colorCode != "" {
			// Try to find -light version for colorization (always SVG)
			lightURL := config.JSDelivrURL + "/svg/" + iconName + "-light.svg"
			if urlExists(lightURL) {
				iconContent, err = fetchRemoteFile(lightURL)
				if err == nil {
					iconContent = applySVGColor(iconContent, colorCode)
				}
			}
		} else {
			// No color - try to serve standard format
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
		
		// Fall back to SVG if standard format not found
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

	// Cache the result
	cache.Set(cacheKey, iconContent)

	log.Printf("[SUCCESS] Serving icon: \"%s\"%s (%s, source: %s)", iconName,
		func() string { if colorCode != "" { return " with color " + colorCode } else { return "" } }(),
		formatToServe, config.IconSource)

	w.Header().Set("Content-Type", contentType)
	w.Write([]byte(iconContent))
}

func handleLegacyIcon(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	colorQuery := r.URL.Query().Get("color")

	var colorCode string
	if colorQuery != "" {
		cleanColor := strings.TrimPrefix(colorQuery, "#")
		if isValidHexColor(cleanColor) {
			colorCode = cleanColor
		}
	}

	// Redirect internally to new handler
	vars["colorcode"] = colorCode
	handleIcon(w, r)
}

func handleCustomIcon(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]

	if filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	// Build the path to the custom icon file
	customPath := filepath.Join("/app/icons/custom", filename)

	// Debug logging
	log.Printf("[DEBUG] Looking for custom icon at: %s", customPath)

	// Check if file exists
	if !fileExists(customPath) {
		// List directory contents for debugging
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

	// Read the file
	data, err := os.ReadFile(customPath)
	if err != nil {
		log.Printf("[ERROR] Failed to read custom icon \"%s\": %v", filename, err)
		http.Error(w, "Failed to read custom icon", http.StatusInternalServerError)
		return
	}

	// Determine content type based on file extension
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

	r := mux.NewRouter()
	
	// Custom icons route: /custom/filename
	r.HandleFunc("/custom/{filename}", handleCustomIcon).Methods("GET")
	
	// Main route: /iconname or /iconname/colorcode
	r.HandleFunc("/{iconname}", handleIcon).Methods("GET")
	r.HandleFunc("/{iconname}/{colorcode}", handleIcon).Methods("GET")
	
	// Legacy route: /iconname.svg?color=colorcode
	r.HandleFunc("/{iconname}.svg", handleLegacyIcon).Methods("GET")
	
	// Root endpoint
	r.HandleFunc("/", handleRoot).Methods("GET")

	log.Printf("Icon server listening on port %s", config.Port)
	log.Printf("Icon source: %s", func() string {
		if config.IconSource == "local" {
			return "Local volume"
		}
		return "Remote CDN"
	}())
	log.Printf("Cache settings: TTL %ds, Max %d items", int(config.CacheTTL.Seconds()), config.CacheSize)

	log.Fatal(http.ListenAndServe(":"+config.Port, r))
}