package main

import (
	"bufio"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/ozdotdotdot/sonotui/internal/daemon"
)

//go:embed web
var webFS embed.FS

// Config holds the daemon configuration.
type Config struct {
	APIPort          int
	FilePort         int
	LanIP            string
	LibraryPath      string
	CachePath        string
	PreferredSpeaker string
	DisplayName      string
}

func defaultConfig() Config {
	home := os.Getenv("HOME")
	return Config{
		APIPort:     8989,
		FilePort:    8990,
		LibraryPath: filepath.Join(home, "Music"),
		CachePath:   filepath.Join(home, ".cache", "sonotuid", "library.json"),
	}
}

func loadConfig(path string) Config {
	cfg := defaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()

	var section string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)

		switch section + "." + k {
		case "server.api_port":
			fmt.Sscan(v, &cfg.APIPort)
		case "server.file_port":
			fmt.Sscan(v, &cfg.FilePort)
		case "server.lan_ip":
			cfg.LanIP = v
		case "library.path":
			cfg.LibraryPath = expandHome(v)
		case "library.cache":
			cfg.CachePath = expandHome(v)
		case "sonos.preferred_speaker":
			cfg.PreferredSpeaker = v
		case "server.display_name":
			cfg.DisplayName = v
		}
	}
	return cfg
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home := os.Getenv("HOME")
		return filepath.Join(home, p[2:])
	}
	return p
}

const systemdUnit = `[Unit]
Description=sonotuid — Sonos daemon
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/sonotuid
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

func installSystemd() error {
	home := os.Getenv("HOME")
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", unitDir, err)
	}
	unitPath := filepath.Join(unitDir, "sonotuid.service")
	if err := os.WriteFile(unitPath, []byte(systemdUnit), 0o644); err != nil {
		return fmt.Errorf("write unit: %w", err)
	}
	cmds := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "sonotuid"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(args, " "), err)
		}
	}
	fmt.Println("sonotuid service installed and started.")
	return nil
}

func main() {
	var (
		flagInstall = flag.Bool("install", false, "install systemd user service and exit")
		flagConfig  = flag.String("config", "", "path to config file")
		flagDebug   = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	if *flagDebug {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	} else {
		log.SetOutput(os.Stderr)
	}

	if *flagInstall {
		if err := installSystemd(); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Find config file.
	cfgPath := *flagConfig
	if cfgPath == "" {
		home := os.Getenv("HOME")
		cfgPath = filepath.Join(home, ".config", "sonotuid", "config.toml")
	}
	cfg := loadConfig(cfgPath)

	// Auto-detect LAN IP if not configured.
	if cfg.LanIP == "" {
		ip, err := daemon.FindLanIP()
		if err != nil {
			log.Printf("warning: could not detect LAN IP: %v", err)
			ip = "127.0.0.1"
		}
		cfg.LanIP = ip
	}
	log.Printf("daemon: LAN IP=%s, API port=%d, file port=%d", cfg.LanIP, cfg.APIPort, cfg.FilePort)
	log.Printf("daemon: library=%s, cache=%s", cfg.LibraryPath, cfg.CachePath)

	// ── Initialise subsystems ────────────────────────────────────────────────

	state := daemon.NewState()
	events := daemon.NewBroadcaster()

	// Library.
	lib := daemon.NewLibrary(cfg.LibraryPath, cfg.CachePath)

	// Sonos manager.
	sonosMgr := daemon.NewSonosManager(state, events, lib, cfg.LanIP, cfg.FilePort, cfg.PreferredSpeaker)

	// File server on :8990.
	fileHandler := daemon.NewFileServer(cfg.LibraryPath, lib).Handler()
	fileServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.FilePort),
		Handler: fileHandler,
	}

	// REST API on :8989.
	api := daemon.NewAPI(state, events, sonosMgr, lib, cfg.LanIP, cfg.FilePort)
	if subFS, err := fs.Sub(webFS, "web"); err == nil {
		api.SetWebFS(subFS)
	} else {
		log.Printf("warning: could not attach web UI: %v", err)
	}
	apiServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.APIPort),
		Handler: api.Handler(),
	}

	// ── Start ────────────────────────────────────────────────────────────────

	// Start file server.
	go func() {
		log.Printf("file server listening on :%d", cfg.FilePort)
		if err := fileServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("file server error: %v", err)
		}
	}()

	// Start API server.
	go func() {
		log.Printf("API server listening on :%d", cfg.APIPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("api server error: %v", err)
		}
	}()

	// mDNS/Bonjour advertisement so iOS (and other mDNS clients) can discover the daemon.
	mdnsName := cfg.DisplayName
	if mdnsName == "" {
		mdnsName, _ = os.Hostname()
	}
	mdnsSrv, err := zeroconf.Register(mdnsName, "_sonogui._tcp", "local.", cfg.APIPort, []string{"version=1"}, nil)
	if err != nil {
		log.Printf("mDNS advertisement failed (non-fatal): %v", err)
	} else {
		log.Printf("mDNS: advertising %q on port %d", mdnsName, cfg.APIPort)
		defer mdnsSrv.Shutdown()
	}

	// Start Sonos manager (discovery + GENA).
	if err := sonosMgr.Start(); err != nil {
		log.Printf("sonos manager: %v", err)
	}

	// Start library scan.
	go lib.Scan(events)

	// ── Shutdown on signal ───────────────────────────────────────────────────

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	sonosMgr.Shutdown()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	apiServer.Shutdown(shutCtx)  //nolint:errcheck
	fileServer.Shutdown(shutCtx) //nolint:errcheck
	log.Println("done.")
}
