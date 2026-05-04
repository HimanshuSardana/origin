package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.mau.fi/whatsmeow/types/events"
)

type Contact struct {
	JID      string
	Name     string
	Preview  string
	Time     string
	Unread   int
}

type Message struct {
	Index    int
	Display  string
	FullMsg  *events.Message
	Time     string
	Type     string
	Sender   string
}

// showContactPicker displays a styled contact list
func showContactPicker(contactList []Contact, onSelect func(Contact)) {
	app := tview.NewApplication()
	
	// Title
	title := tview.NewTextView().
		SetText(" WhatsApp Contacts ").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorWhite).
		SetBackgroundColor(tcell.ColorDarkGreen)

	// Contact list
	list := tview.NewList().
		ShowSecondaryText(true).
		SetSelectedBackgroundColor(tcell.ColorDarkGreen).
		SetSelectedTextColor(tcell.ColorWhite).
		SetMainTextColor(tcell.ColorWhite).
		SetSecondaryTextColor(tcell.ColorGray)

	for _, c := range contactList {
		name := c.Name
		if c.Unread > 0 {
			name = fmt.Sprintf("[yellow]%s[-] [%s]%d[-]", c.Name, "red", c.Unread)
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

	// Help text
	help := tview.NewTextView().
		SetText(" ↑/↓: Navigate | Enter: Select | Esc: Exit ").
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
			onSelect(Contact{})
			return nil
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
		SetText(fmt.Sprintf(" Messages (%d) ", len(messages))).
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

		mainText := fmt.Sprintf("[%s]%s %s[-]", color, prefix, msg.Display)
		secondaryText := fmt.Sprintf("   [gray]%s | %s[-]", msg.Time, msg.Sender)
		
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

// formatMessagePreview creates a preview string for a message
func formatMessagePreview(msg *events.Message) string {
	if conv := msg.Message.GetConversation(); conv != "" {
		preview := strings.ReplaceAll(conv, "\n", " ")
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		return preview
	}
	if msg.Message.GetImageMessage() != nil {
		return "📷 Image"
	}
	if msg.Message.GetVideoMessage() != nil {
		return "🎥 Video"
	}
	if msg.Message.GetDocumentMessage() != nil {
		return "📎 Document"
	}
	return "📎 Media"
}
