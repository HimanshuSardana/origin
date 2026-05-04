package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/HimanshuSardana/origin/whatsapp"
	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	sidebar    sidebarModel
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
	return m.sidebar.Init()
}

func (m model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		m.sidebar = newSidebar.(sidebarModel)

		// Check if a contact was selected (Enter was pressed)
		if m.sidebar.selected >= 0 && m.sidebar.selected < len(m.sidebar.items) {
			selectedContact := m.sidebar.items[m.sidebar.selected]
			m.chatView.contactJID = selectedContact.JID
			m.sidebar.selected = -1 // reset

			// Return a command to load messages
			return m, func() bubbletea.Msg {
				messages, err := m.waClient.GetMessages(context.Background(), selectedContact.JID, 10)
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

// sidebarModel represents the contact list with scrolling
type sidebarModel struct {
	items          []whatsapp.Contact
	cursor         int
	selected       int // -1 means none
	viewportHeight int // number of items visible at once
	scrollOffset   int // index of first visible item
}

func initialSidebar(waClient *whatsapp.Client) sidebarModel {
	contacts, err := waClient.GetContacts()
	if err != nil {
		return sidebarModel{items: []whatsapp.Contact{}}
	}
	return sidebarModel{
		items:          contacts,
		cursor:         0,
		selected:       -1,
		viewportHeight: 20, // show 20 contacts at a time
		scrollOffset:   0,
	}
}

func (m sidebarModel) Init() bubbletea.Cmd { return nil }

func (m sidebarModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
				// Adjust scroll offset if cursor goes above viewport
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}
		case "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				// Adjust scroll offset if cursor goes below viewport
				if m.cursor >= m.scrollOffset+m.viewportHeight {
					m.scrollOffset = m.cursor - m.viewportHeight + 1
				}
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				m.selected = m.cursor
			}
		}
	}
	return m, nil
}

func (m sidebarModel) View() string {
	var s strings.Builder
	
	// Show scroll up indicator if not at top
	if m.scrollOffset > 0 {
		s.WriteString("  ▲ more above\n")
	}
	
	// Calculate visible range
	start := m.scrollOffset
	end := start + m.viewportHeight
	if end > len(m.items) {
		end = len(m.items)
	}
	
	// Render visible items
	for i := start; i < end; i++ {
		item := m.items[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		
		// Truncate long names
		name := item.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}
		
		s.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
	}
	
	// Show scroll down indicator if not at bottom
	if end < len(m.items) {
		s.WriteString(fmt.Sprintf("  ▼ %d more below\n", len(m.items)-end))
	}
	
	// Show counter at bottom
	if len(m.items) > 0 {
		s.WriteString(fmt.Sprintf("\n  [%d/%d]\n", m.cursor+1, len(m.items)))
	} else {
		s.WriteString("No contacts found\n")
	}
	
	return s.String()
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
