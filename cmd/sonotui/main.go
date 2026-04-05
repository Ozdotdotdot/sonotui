package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ozdotdotdot/sonotui/config"
	clienttui "github.com/ozdotdotdot/sonotui/internal/tui"
)

func main() {
	var (
		flagHost         = flag.String("host", "127.0.0.1", "sonotuid host")
		flagPort         = flag.Int("port", 8989, "sonotuid API port")
		flagDebug        = flag.Bool("debug", false, "enable debug logging")
		flagStatusSecs   = flag.Int("status-seconds", 4, "seconds to show transient status messages")
		flagArtProto     = flag.String("art-protocol", "", "terminal image protocol: kitty, sixel, none (default: auto-detect)")
	)
	flag.Parse()

	if *flagDebug {
		cleanup := config.SetupDebugLog()
		defer cleanup()
	}

	addr := clienttui.DaemonAddr{
		Host: *flagHost,
		Port: *flagPort,
	}

	artProto := clienttui.DetectProtocol()
	if *flagArtProto != "" {
		if p, ok := clienttui.ParseProtocol(*flagArtProto); ok {
			artProto = p
		} else {
			fmt.Fprintf(os.Stderr, "unknown art-protocol %q, using auto-detect\n", *flagArtProto)
		}
	}

	m := clienttui.NewModel(addr, artProto)
	if *flagStatusSecs > 0 {
		m.SetStatusTTL(time.Duration(*flagStatusSecs) * time.Second)
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
