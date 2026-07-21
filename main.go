package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/CooDdk/freexnats/internal/app"
	"github.com/CooDdk/freexnats/internal/config"
)

func main() {
	var (
		showVersion bool
		showHelp    bool
		noSplash    bool
		natsURL     string
		natsUser    string
		natsPass    string
		natsToken   string
	)

	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showHelp, "h", false, "Show help information (shorthand)")
	flag.BoolVar(&noSplash, "no-splash", false, "Skip splash screen on startup")
	flag.StringVar(&natsURL, "url", "", "NATS server URL (e.g. nats://remote:4222)")
	flag.StringVar(&natsUser, "user", "", "NATS username for authentication")
	flag.StringVar(&natsPass, "pass", "", "NATS password for authentication")
	flag.StringVar(&natsToken, "token", "", "NATS token for authentication")

	flag.Parse()

	if showVersion {
		fmt.Printf("%s %s\n", config.AppName, config.AppVersion)
		fmt.Println(config.AppDesc)
		os.Exit(0)
	}

	if showHelp {
		printHelp()
		os.Exit(0)
	}

	m := app.NewModel(natsURL, natsUser, natsPass, natsToken)

	if noSplash {
		m = m.WithNoSplash()
	}

	zone.NewGlobal()

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf("%s - %s\n\n", config.AppName, config.AppDesc)
	fmt.Printf("Version: %s\n\n", config.AppVersion)
	fmt.Println("Usage:")
	fmt.Printf("  %s [flags]\n\n", config.AppName)
	fmt.Println("Flags:")
	fmt.Println("  -h, --help       Show help information")
	fmt.Println("  -v, --version    Show version information")
	fmt.Println("      --no-splash  Skip splash screen on startup")
	fmt.Println("      --url        NATS server URL (default: nats://localhost:4222)")
	fmt.Println("      --user       NATS username for authentication")
	fmt.Println("      --pass       NATS password for authentication")
	fmt.Println("      --token      NATS token for authentication")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Printf("  %s                                  # Connect to local NATS\n", config.AppName)
	fmt.Printf("  %s --url nats://remote:4222         # Connect to remote NATS\n", config.AppName)
	fmt.Printf("  %s --url nats://remote:4222 --user admin --pass secret\n", config.AppName)
}
