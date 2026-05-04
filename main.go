package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/HimanshuSardana/origin/whatsapp"
)

func main() {
	listFlag := flag.Bool("list", false, "List all contacts in fzf picker")
	tuiFlag := flag.Bool("tui", false, "Open interactive TUI")
	jidFlag := flag.String("jid", "", "Directly specify JID for testing")
	flag.Parse()

	// Only one mode at a time
	if *listFlag && *tuiFlag {
		fmt.Fprintf(os.Stderr, "Error: cannot use both --list and --tui\n")
		os.Exit(1)
	}

	if *tuiFlag {
		// Bubbletea TUI mode
		client, err := whatsapp.NewClient("origin.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		if err := runTUI(client); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *listFlag {
		// FZF mode - pick contact, then pick message, then copy/download
		if err := runListMode(*jidFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Normal mode
	fmt.Println("Usage:")
	fmt.Println("  ./origin --list     # Pick contact and message using fzf")
	fmt.Println("  ./origin --tui      # Interactive TUI")
	fmt.Println("  ./origin            # Normal WhatsApp client")
}
