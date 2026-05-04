package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
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
	"google.golang.org/protobuf/proto"
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
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan QR code with WhatsApp to continue...")
			}
		}
	} else {
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}
	defer client.Disconnect()

	fmt.Println("Connected! Fetching contacts...")

	// Get all contacts from DB
	db, err := sql.Open("sqlite3", "file:origin.db?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT their_jid, full_name FROM whatsmeow_contacts WHERE full_name != '' ORDER BY full_name")
	if err != nil {
		return fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	type contact struct {
		jid  string
		name string
	}
	var contacts []contact
	for rows.Next() {
		var c contact
		if err := rows.Scan(&c.jid, &c.name); err != nil {
			continue
		}
		contacts = append(contacts, c)
	}

	fmt.Printf("Found %d contacts. Requesting recent messages for each...\n", len(contacts))

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

	// Channel to receive history sync
	historyChan := make(chan *events.HistorySync, 10)

	// Add event handler for history sync
	handlerID := client.AddEventHandler(func(evt interface{}) {
		if hs, ok := evt.(*events.HistorySync); ok {
			if hs.Data.GetSyncType() == waHistorySync.HistorySync_ON_DEMAND {
				select {
				case historyChan <- hs:
				default:
				}
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	totalMessages := 0
	processedContacts := 0

	for _, c := range contacts {
		processedContacts++
		fmt.Printf("[%d/%d] Processing %s (%s)...\n", processedContacts, len(contacts), c.name, c.jid)

		jid, err := types.ParseJID(c.jid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid JID %s: %v\n", c.jid, err)
			continue
		}

		// Request history sync for this contact
		req := &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
				PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
					PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND.Enum(),
					HistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_HistorySyncOnDemandRequest{
						ChatJID:          proto.String(jid.String()),
						OnDemandMsgCount: proto.Int32(50),
					},
				},
			},
		}

		_, err = client.SendPeerMessage(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to request history for %s: %v\n", c.jid, err)
			continue
		}

		// Wait for response with timeout
		select {
		case hs := <-historyChan:
			// Find the conversation for this JID
			for _, conv := range hs.Data.GetConversations() {
				if conv.GetID() == jid.String() || conv.GetID() == jid.ToNonAD().String() {
					msgCount := 0
					for _, histMsg := range conv.GetMessages() {
						parsedMsg, err := client.ParseWebMessage(jid, histMsg.GetMessage())
						if err != nil {
							continue
						}

						var content string
						var msgType string
						if conv := parsedMsg.Message.GetConversation(); conv != "" {
							content = strings.TrimSpace(conv)
							msgType = "text"
						} else if parsedMsg.Message.GetImageMessage() != nil {
							content = "[Image]"
							msgType = "image"
						} else if parsedMsg.Message.GetVideoMessage() != nil {
							content = "[Video]"
							msgType = "video"
						} else if parsedMsg.Message.GetDocumentMessage() != nil {
							content = "[Document]"
							msgType = "document"
						} else {
							content = "[Other]"
							msgType = "other"
						}

						// Truncate content
						if len(content) > 500 {
							content = content[:500] + "..."
						}

						_, err = db.Exec(`INSERT OR IGNORE INTO whatsmeow_messages 
							(chat_jid, message_id, sender_jid, timestamp, content, message_type) 
							VALUES (?, ?, ?, ?, ?, ?)`,
							c.jid,
							parsedMsg.Info.ID,
							parsedMsg.Info.Sender.String(),
							parsedMsg.Info.Timestamp,
							content,
							msgType,
						)
						if err == nil {
							msgCount++
							totalMessages++
						}
					}
					fmt.Printf("  Stored %d messages\n", msgCount)
					break
				}
			}
		case <-time.After(10 * time.Second):
			fmt.Fprintf(os.Stderr, "  Timeout waiting for response from %s\n", c.jid)
		}

		// Small delay to avoid rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("\nDone! Total messages stored: %d\n", totalMessages)
	return nil
}
