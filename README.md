# Origin - WhatsApp Terminal UI

A terminal-based WhatsApp client built in Go using whatsmeow and tview. Features an interactive TUI for browsing contacts and messages, with media download and clipboard integration.

## Features

### Interactive TUI
- **Contact Picker** - Browse all WhatsApp contacts with real-time search
- **Message Viewer** - View last 10 messages from any contact
- **Message Types** - Supports text, images, videos, and documents
- **Keyboard Navigation** - Full keyboard and mouse support

### Core Functionality
- **History Sync** - Fetches message history on-demand from WhatsApp
- **Media Downloads** - Save images/videos/docs with custom filenames
- **Clipboard Integration** - Copy text messages directly to clipboard (xclip)
- **Contact Search** - Press `/` to search contacts by name or JID

## Installation

### Prerequisites
- Go 1.21+
- SQLite3
- fzf (optional, used for initial contact browsing)
- xclip (for clipboard functionality)
- tview dependencies (automatically installed)

### Build

```bash
git clone https://github.com/HimanshuSardana/origin
cd origin
go mod tidy
make build
```

## Usage

### Basic Commands

```bash
./origin --list

./origin --list --jid="919899004405@s.whatsapp.net"

# (QR login + send test message)
./origin
```

## Notes

- First run requires QR code scan with WhatsApp mobile app
- History sync fetches last 10 messages per contact on-demand
- Messages are parsed from WhatsApp's protobuf format
- Media downloads use WhatsApp's media servers

## Roadmap
- [ ] Group chat support
- [ ] Message sending functionality
- [ ] Better TUI (bubbletea)
