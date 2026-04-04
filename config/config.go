package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config holds persistent settings for sonotui.
type Config struct {
	LastIP   string
	LastName string
}

// configDir returns the XDG_CONFIG_HOME/sonotui directory, creating it if needed.
func configDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "sonotui")
}

// cacheDir returns the XDG_CACHE_HOME/sonotui directory, creating it if needed.
func CacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "sonotui")
}

// Load reads the config file. Returns empty Config if the file doesn't exist.
func Load() Config {
	path := filepath.Join(configDir(), "config.toml")
	f, err := os.Open(path)
	if err != nil {
		return Config{}
	}
	defer f.Close()

	cfg := Config{}
	var inSpeaker bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[speaker]" {
			inSpeaker = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSpeaker = false
			continue
		}
		if !inSpeaker {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch k {
		case "last_ip":
			cfg.LastIP = v
		case "last_name":
			cfg.LastName = v
		}
	}
	return cfg
}

// Save writes the config to disk.
func Save(cfg Config) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("config: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, "config.toml")
	content := fmt.Sprintf("[speaker]\nlast_ip   = %q\nlast_name = %q\n", cfg.LastIP, cfg.LastName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Printf("config: write %s: %v", path, err)
	}
}

// SetupDebugLog sets up a debug log file in the cache directory.
// Returns a cleanup function to call at exit.
func SetupDebugLog() func() {
	dir := CacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	path := filepath.Join(dir, "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return func() { f.Close() }
}
