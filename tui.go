package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/HimanshuSardana/origin/whatsapp"
	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// contactItem implements list.Item for the bubbles list component
type contactItem struct {
	contact whatsapp.Contact
}

func (i contactItem) Title() string       { return i.contact.Name }
func (i contactItem) Description() string { return i.contact.JID }
func (i contactItem) FilterValue() string { return i.contact.Name }

// Panel identifiers
type panelType int

const (
	panelSidebar panelType = iota
	panelChat
	panelInput
)

// Model represents the entire TUI state
type model struct {
	// Components
	sidebar    list.Model
	chatView   chatViewModel
	inputField inputModel

	// State
	activePanel panelType
	width       int
	height      int

	// WhatsApp client
	waClient *whatsapp.Client
}

func initialModel(waClient *whatsapp.Client) model {
	return model{
		sidebar:     initialSidebar(waClient),
		chatView:    initialChatView(),
		inputField:  initialInputField(),
		activePanel: panelSidebar,
		waClient:    waClient,
	}
}

func (m model) Init() bubbletea.Cmd {
	return nil
}

func (m model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update sidebar size
		m.sidebar.SetSize(m.width/3, m.height)
		return m, nil

	case messagesLoadedMsg:
		if msg.err != nil {
			// Error loading messages - could show error in UI
			return m, nil
		}
		m.chatView.messages = msg.messages
		// Auto-switch to chat view when messages load
		m.activePanel = panelChat
		return m, nil

	case bubbletea.KeyMsg:
		// Global key bindings for panel navigation
		switch msg.String() {
		case "1":
			m.activePanel = panelSidebar
			return m, nil
		case "2":
			m.activePanel = panelChat
			return m, nil
		case "3":
			m.activePanel = panelInput
			return m, nil
		case "q", "ctrl+c":
			return m, bubbletea.Quit
		}
	}

	// Route key events to active panel
	switch m.activePanel {
	case panelSidebar:
		newSidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = newSidebar

		// Check if a contact was selected (Enter was pressed)
		if sel := m.sidebar.SelectedItem(); sel != nil && msg.(bubbletea.KeyMsg).String() == "enter" {
			item := sel.(contactItem)
			m.chatView.contactJID = item.contact.JID

			// Return a command to load messages
			return m, func() bubbletea.Msg {
				messages, err := m.waClient.GetMessages(context.Background(), item.contact.JID, 10)
				if err != nil {
					return messagesLoadedMsg{err: err}
				}
				return messagesLoadedMsg{messages: messages}
			}
		}

		return m, cmd
	case panelChat:
		newChat, cmd := m.chatView.Update(msg)
		m.chatView = newChat.(chatViewModel)
		return m, cmd
	case panelInput:
		newInput, cmd := m.inputField.Update(msg)
		m.inputField = newInput.(inputModel)

		// Check if message was sent
		if m.inputField.sent {
			// Send message to selected contact
			if m.chatView.contactJID != "" {
				go func() {
					m.waClient.SendMessage(context.Background(), m.chatView.contactJID, m.inputField.value)
				}()
				m.inputField.value = ""
				m.inputField.sent = false
			}
		}

		return m, cmd
	}

	return m, nil
}

// messagesLoadedMsg is sent when messages are loaded from WhatsApp
type messagesLoadedMsg struct {
	messages []whatsapp.Message
	err      error
}

func (m model) View() string {
	// Define layout
	sidebarWidth := m.width / 3
	chatWidth := m.width - sidebarWidth

	// Render panels
	sidebarView := m.sidebar.View()
	chatView := m.chatView.View()
	inputView := m.inputField.View()

	// Highlight active panel
	if m.activePanel == panelSidebar {
		sidebarView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Render(sidebarView)
	} else {
		sidebarView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(sidebarView)
	}

	if m.activePanel == panelChat {
		chatView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Render(chatView)
	} else {
		chatView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(chatView)
	}

	if m.activePanel == panelInput {
		inputView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Render(inputView)
	} else {
		inputView = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(inputView)
	}

	// Layout: sidebar | (chat + input)
	rightPanel := lipgloss.JoinVertical(lipgloss.Top, chatView, inputView)

	return lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Width(sidebarWidth).Height(m.height).Render(sidebarView),
		lipgloss.NewStyle().Width(chatWidth).Height(m.height).Render(rightPanel),
	)
}

func initialSidebar(waClient *whatsapp.Client) list.Model {
	contacts, err := waClient.GetContacts()
	if err != nil {
		contacts = []whatsapp.Contact{}
	}

	// Convert contacts to list items
	items := make([]list.Item, len(contacts))
	for i, c := range contacts {
		items[i] = contactItem{contact: c}
	}

	// Create the list with default delegate
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Contacts"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	return l
}

// chatViewModel represents the chat history view
type chatViewModel struct {
	messages   []whatsapp.Message
	contactJID string
}

func initialChatView() chatViewModel {
	return chatViewModel{
		messages:   []whatsapp.Message{},
		contactJID: "",
	}
}

func (m chatViewModel) Init() bubbletea.Cmd { return nil }

func (m chatViewModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	return m, nil
}

func (m chatViewModel) View() string {
	var s strings.Builder

	if m.contactJID == "" {
		s.WriteString("Select a contact to view chat\n")
		return s.String()
	}

	if len(m.messages) == 0 {
		s.WriteString("No messages yet\n")
		return s.String()
	}

	for _, msg := range m.messages {
		s.WriteString(fmt.Sprintf("[%s] %s: %s\n", msg.Time, msg.Sender, msg.Display))
	}

	return s.String()
}

// inputModel represents the message input field
type inputModel struct {
	value string
	sent  bool
}

func initialInputField() inputModel {
	return inputModel{
		value: "",
		sent:  false,
	}
}

func (m inputModel) Init() bubbletea.Cmd { return nil }

func (m inputModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.Type {
		case bubbletea.KeyEnter:
			if m.value != "" {
				m.sent = true
			}
		case bubbletea.KeyRunes:
			m.value += msg.String()
		case bubbletea.KeyBackspace:
			if len(m.value) > 0 {
				m.value = m.value[:len(m.value)-1]
			}
		}
	}
	return m, nil
}

func (m inputModel) View() string {
	return fmt.Sprintf("> %s", m.value)
}

// runTUI starts the Bubbletea TUI
func runTUI(client *whatsapp.Client) error {
	p := bubbletea.NewProgram(initialModel(client), bubbletea.WithAltScreen())
	_, err := p.Run()
	return err
}
