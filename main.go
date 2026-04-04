package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ozdotdotdot/sonotui/config"
	"github.com/ozdotdotdot/sonotui/sonos"
	"github.com/ozdotdotdot/sonotui/tui"
)

func main() {
	var (
		flagSpeaker = flag.String("speaker", "", "skip discovery, connect to this IP directly")
		flagPort    = flag.Int("port", 0, "override notify server port (default: auto 34500–34599)")
		flagDebug   = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	if *flagDebug {
		cleanup := config.SetupDebugLog()
		defer cleanup()
	} else {
		// Silence the standard logger unless debug mode is on.
		log.SetOutput(os.Stderr)
		log.SetFlags(0)
	}

	artProto := tui.DetectProtocol()

	m := tui.NewModel(artProto, *flagPort)

	// If a speaker IP was provided directly, pre-populate and skip discovery wait.
	if *flagSpeaker != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		sp, err := sonos.FetchSpeakerByIP(ctx, *flagSpeaker)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not reach speaker at %s: %v\n", *flagSpeaker, err)
			// Fall through to discovery.
		} else {
			m = tui.NewModelWithSpeakers(artProto, *flagPort, []sonos.Speaker{sp})
		}
	} else {
		// Try saved IP from config while discovery runs.
		cfg := config.Load()
		if cfg.LastIP != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if sp, err := sonos.FetchSpeakerByIP(ctx, cfg.LastIP); err == nil {
				m = tui.NewModelWithSpeakers(artProto, *flagPort, []sonos.Speaker{sp})
			}
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Save last speaker to config.
	if fm, ok := finalModel.(tui.Model); ok {
		if sp := fm.ActiveSpeaker(); sp != nil {
			config.Save(config.Config{
				LastIP:   sp.IP,
				LastName: sp.FriendlyName,
			})
		}
	}
}
