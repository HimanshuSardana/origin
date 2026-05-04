package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	qrterminal "github.com/mdp/qrterminal"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func runReload() error {
	fmt.Println("Starting reload of recent messages...")

	dbLog := waLog.Stdout("Database", "ERROR", false)
	ctx := context.Background()

	// Open SQLite database
	container, err := sqlstore.New(ctx, "sqlite3", "file:origin.db?_foreign_keys=on", dbLog)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	// Get device store
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	// Create client
	clientLog := waLog.Stdout("Client", "ERROR", false)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// Connect to WhatsApp
	if client.Store.ID == nil {
		// Need to log in
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan QR code with WhatsApp to continue...")
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}
	defer client.Disconnect()

	fmt.Println("Connected! Requesting full history sync...")

	// Channel to receive history sync
	historyChan := make(chan *events.HistorySync, 1)

	// Add event handler for history sync
	handlerID := client.AddEventHandler(func(evt interface{}) {
		if hs, ok := evt.(*events.HistorySync); ok {
			fmt.Fprintf(os.Stderr, "DEBUG: Got HistorySync event, type=%v\n", hs.Data.GetSyncType())
			// Accept ON_DEMAND sync (response to our request)
			if hs.Data.GetSyncType() == waHistorySync.HistorySync_ON_DEMAND {
				select {
				case historyChan <- hs:
					fmt.Fprintf(os.Stderr, "DEBUG: Sent history to channel\n")
				default:
					fmt.Fprintf(os.Stderr, "DEBUG: Channel full, dropping\n")
				}
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	// Request full history sync using FULL_HISTORY_SYNC_ON_DEMAND
	req := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
			},
		},
	}

	_, err = client.SendPeerMessage(context.Background(), req)
	if err != nil {
		return fmt.Errorf("send history sync request: %w", err)
	}

	fmt.Println("Waiting for history sync response (up to 60 seconds)...")

	// Wait for history sync with timeout
	select {
	case hs := <-historyChan:
		fmt.Printf("Received history sync with %d conversations\n", len(hs.Data.GetConversations()))

		// Open direct DB connection for storing messages
		db, err := sql.Open("sqlite3", "file:origin.db?_foreign_keys=on")
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer db.Close()

		// Create messages table if not exists
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS whatsmeow_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_jid TEXT NOT NULL,
			message_id TEXT NOT NULL,
			sender_jid TEXT,
			timestamp DATETIME,
			content TEXT,
			message_type TEXT,
			UNIQUE(chat_jid, message_id)
		)`)
		if err != nil {
			return fmt.Errorf("create table: %w", err)
		}

		// Process conversations
		totalMessages := 0
		for _, conv := range hs.Data.GetConversations() {
			chatJID := conv.GetID()
			fmt.Printf("Processing chat %s with %d messages...\n", chatJID, len(conv.GetMessages()))

			for _, histMsg := range conv.GetMessages() {
				parsedMsg, err := client.ParseWebMessage(types.JID{}, histMsg.GetMessage())
				if err != nil {
					continue
				}

				// Extract message content
				var content string
				var msgType string
				if conv := parsedMsg.Message.GetConversation(); conv != "" {
					content = conv
					msgType = "text"
				} else if img := parsedMsg.Message.GetImageMessage(); img != nil {
					content = "[Image]"
					msgType = "image"
				} else if vid := parsedMsg.Message.GetVideoMessage(); vid != nil {
					content = "[Video]"
					msgType = "video"
				} else if doc := parsedMsg.Message.GetDocumentMessage(); doc != nil {
					content = "[Document]"
					msgType = "document"
				} else {
					content = "[Other]"
					msgType = "other"
				}

				// Store in database
				_, err = db.Exec(`INSERT OR IGNORE INTO whatsmeow_messages 
					(chat_jid, message_id, sender_jid, timestamp, content, message_type) 
					VALUES (?, ?, ?, ?, ?, ?)`,
					chatJID,
					parsedMsg.Info.ID,
					parsedMsg.Info.Sender.String(),
					parsedMsg.Info.Timestamp,
					content,
					msgType,
				)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error storing message: %v\n", err)
				} else {
					totalMessages++
				}
			}
		}

		fmt.Printf("Successfully stored %d messages in database\n", totalMessages)
		return nil

	case <-time.After(60 * time.Second):
		return fmt.Errorf("timeout waiting for history sync response")
	}
}
