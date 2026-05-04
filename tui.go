package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.mau.fi/whatsmeow/types/events"
)

type Contact struct {
	JID     string
	Name    string
	Preview string
	Time    string
	Unread  int
}

type Message struct {
	Index   int
	Display string
	FullMsg *events.Message
	Time    string
	Type    string
	Sender  string
}

// showContactPicker displays a styled contact list with search
func showContactPicker(contactList []Contact, onSelect func(Contact)) {
	app := tview.NewApplication()

	// Title
	title := tview.NewTextView().
		SetText(" WhatsApp Contacts ").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorWhite).
		SetBackgroundColor(tcell.ColorDarkGreen)

	// Search input (initially hidden)
	searchInput := tview.NewInputField().
		SetLabel("Search: ").
		SetFieldBackgroundColor(tcell.ColorDarkGray).
		SetFieldTextColor(tcell.ColorWhite)

	// Contact list
	list := tview.NewList().
		ShowSecondaryText(true).
		SetSelectedBackgroundColor(tcell.ColorDarkGreen).
		SetSelectedTextColor(tcell.ColorWhite).
		SetMainTextColor(tcell.ColorWhite).
		SetSecondaryTextColor(tcell.ColorGray)

	// Store all contacts for filtering
	allContacts := contactList
	filteredContacts := contactList

	// Function to refresh list with filtered contacts
	refreshList := func(filter string) {
		list.Clear()
		filter = strings.ToLower(strings.TrimSpace(filter))
		
		if filter == "" {
			filteredContacts = allContacts
		} else {
			filteredContacts = nil
			for _, c := range allContacts {
				if strings.Contains(strings.ToLower(c.Name), filter) ||
					strings.Contains(strings.ToLower(c.JID), filter) {
					filteredContacts = append(filteredContacts, c)
				}
			}
		}

		for _, c := range filteredContacts {
			name := c.Name
			if c.Unread > 0 {
				name = "[" + c.Name + "] [red]" + string(rune(c.Unread+'0')) + "[-]"
			}
			preview := c.Preview
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			contact := c
			list.AddItem(name, preview, 0, func() {
				app.Stop()
				onSelect(contact)
			})
		}
	}

	// Initial population
	refreshList("")

	// Current focus: "list" or "search"
	focus := "list"

	// Help text
	help := tview.NewTextView().
		SetText(" ↑/↓: Navigate | Enter: Select | /: Search | Esc: Exit ").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorGray)

	// Layout - search hidden initially
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(help, 1, 0, false)

	// Search input handler
	searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			// Clear search and return to list
			searchInput.SetText("")
			refreshList("")
			focus = "list"
			flex.RemoveItem(searchInput)
			app.SetFocus(list)
			return nil
		}
		if event.Key() == tcell.KeyEnter {
			// Return to list with current filter
			focus = "list"
			flex.RemoveItem(searchInput)
			app.SetFocus(list)
			return nil
		}
		return event
	})

	// Update filter as user types
	searchInput.SetChangedFunc(func(text string) {
		refreshList(text)
	})

	// Main input handler
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if focus == "list" {
			if event.Rune() == '/' {
				// Enter search mode
				focus = "search"
				flex.AddItem(searchInput, 1, 0, false)
				app.SetFocus(searchInput)
				return nil
			}
			if event.Key() == tcell.KeyEscape {
				app.Stop()
				onSelect(Contact{})
				return nil
			}
		}
		return event
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

// showMessagePicker displays messages in a styled list
func showMessagePicker(messages []Message, onSelect func(Message)) {
	app := tview.NewApplication()

	// Title bar
	title := tview.NewTextView().
		SetText(" Messages (" + string(rune(len(messages)+'0')) + ") ").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorWhite).
		SetBackgroundColor(tcell.ColorDarkBlue)

	// Message list with better formatting
	list := tview.NewList().
		ShowSecondaryText(true).
		SetSelectedBackgroundColor(tcell.ColorDarkBlue).
		SetSelectedTextColor(tcell.ColorWhite).
		SetMainTextColor(tcell.ColorWhite).
		SetSecondaryTextColor(tcell.ColorGray)

	for _, msg := range messages {
		// Format based on message type
		prefix := "📄"
		color := "white"
		switch msg.Type {
		case "Text":
			prefix = "💬"
			color = "white"
		case "Image":
			prefix = "🖼️"
			color = "green"
		case "Video":
			prefix = "🎥"
			color = "blue"
		case "Doc":
			prefix = "📎"
			color = "yellow"
		}

		mainText := "[" + color + "]" + prefix + " " + msg.Display + "[-]"
		secondaryText := "   [gray]" + msg.Time + " | " + msg.Sender + "[-]"

		message := msg
		list.AddItem(mainText, secondaryText, 0, func() {
			app.Stop()
			onSelect(message)
		})
	}

	// Help
	help := tview.NewTextView().
		SetText(" ↑/↓: Navigate | Enter: Select | Esc: Back ").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorGray)

	// Layout
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(help, 1, 0, false)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Stop()
			onSelect(Message{})
			return nil
		}
		return event
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
