package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/HimanshuSardana/origin/whatsapp"
	bubbletea "github.com/charmbracelet/bubbletea"
)

func main() {
	listFlag := flag.Bool("list", false, "List all contacts in TUI")
	jidFlag := flag.String("jid", "", "Directly specify JID for testing")
	flag.Parse()

	client, err := whatsapp.NewClient("origin.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if *listFlag {
		// TUI mode
		if err := runTUI(client, *jidFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Normal mode - QR login + send test message
	fmt.Println("Normal mode - use --list for TUI")
}

func runTUI(client *whatsapp.Client, jidFlag string) error {
	p := bubbletea.NewProgram(initialModel(client))
	_, err := p.Run()
	return err
}
