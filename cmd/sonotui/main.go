package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ozdotdotdot/sonotui/config"
	clienttui "github.com/ozdotdotdot/sonotui/internal/tui"
)

func main() {
	var (
		flagHost  = flag.String("host", "127.0.0.1", "sonotuid host")
		flagPort  = flag.Int("port", 8989, "sonotuid API port")
		flagDebug = flag.Bool("debug", false, "enable debug logging")
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

	m := clienttui.NewModel(addr, clienttui.DetectProtocol())
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
