package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
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
	// Panels
	sidebar     bubbletea.Model
	chatView    bubbletea.Model
	inputField  bubbletea.Model

	// State
	activePanel panelType
	width       int
	height      int
}

func initialModel() model {
	return model{
		sidebar:    initialSidebar(),
		chatView:   initialChatView(),
		inputField: initialInputField(),
		activePanel: panelSidebar,
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
		// TODO: update child models with new sizes
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
		return m, cmd
	case panelChat:
		newChat, cmd := m.chatView.Update(msg)
		m.chatView = newChat
		return m, cmd
	case panelInput:
		newInput, cmd := m.inputField.Update(msg)
		m.inputField = newInput
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	// Define layout
	sidebarWidth := m.width / 3
	chatWidth := m.width - sidebarWidth
	inputHeight := 3

	// Adjust for input field
	chatHeight := m.height - inputHeight

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

// Placeholder models for each panel
type sidebarModel struct {
	items    []whatsapp.Contact
	cursor   int
	selected int // -1 means none
}

func initialSidebar() bubbletea.Model {
	return sidebarModel{
		items: []whatsapp.Contact{},
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
			}
		case "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}
func (m sidebarModel) View() string {
	var s strings.Builder
	for i, item := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = "> "
		}
		s.WriteString(fmt.Sprintf("%s%s (%s)\n", cursor, item.Name, item.JID))
	}
	if len(m.items) == 0 {
		s.WriteString("No contacts found\n")
	}
	return s.String()
}

type chatViewModel struct{ content string }
func initialChatView() bubbletea.Model { return chatViewModel{content: "Select a contact to view chat"} }
func (m chatViewModel) Init() bubbletea.Cmd { return nil }
func (m chatViewModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) { return m, nil }
func (m chatViewModel) View() string { return m.content }

type inputModel struct{ value string }
func initialInputField() bubbletea.Model { return inputModel{} }
func (m inputModel) Init() bubbletea.Cmd { return nil }
func (m inputModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.Type {
		case bubbletea.KeyEnter:
			// TODO: send message
			m.value = ""
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
func (m inputModel) View() string { return fmt.Sprintf("> %s", m.value) }
