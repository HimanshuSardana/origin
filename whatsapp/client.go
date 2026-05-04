package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	waClient  *whatsmeow.Client
	db        *sql.DB
	container  *sqlstore.Container
}

type Contact struct {
	JID     string
	Name    string
	Preview string
}

type Message struct {
	Index   int
	Display string
	FullMsg *events.Message
	Time    string
	Type    string
	Sender  string
}

func NewClient(dbPath string) (*Client, error) {
	dbLog := waLog.Stdout("Database", "ERROR", false)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+dbPath+"?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	return &Client{
		container: container,
		db:        db,
	}, nil
}

func (c *Client) GetContacts() ([]Contact, error) {
	rows, err := c.db.Query("SELECT their_jid, full_name FROM whatsmeow_contacts WHERE full_name != '' ORDER BY full_name")
	if err != nil {
		return nil, fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var jid, fullName string
		if err := rows.Scan(&jid, &fullName); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		contacts = append(contacts, Contact{
			JID:     jid,
			Name:    fullName,
			Preview: "Click to view messages...",
		})
	}

	return contacts, nil
}

func (c *Client) GetMessages(ctx context.Context, chatJID string, count int) ([]Message, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return nil, fmt.Errorf("parse JID: %w", err)
	}

	// Get device and connect if needed
	deviceStore, err := c.container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", false)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	if client.Store.ID == nil {
		return nil, fmt.Errorf("client not logged in")
	}

	err = client.Connect()
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect()

	// Request history sync
	historyChan := make(chan *events.HistorySync, 1)
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

	req := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND.Enum(),
				HistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_HistorySyncOnDemandRequest{
					ChatJID:          proto.String(jid.String()),
					OnDemandMsgCount: proto.Int32(int32(count)),
				},
			},
		},
	}
	_, err = client.SendPeerMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send history sync request: %w", err)
	}

	select {
	case hs := <-historyChan:
		var messages []*events.Message
		for _, conv := range hs.Data.GetConversations() {
			if conv.GetID() == jid.String() || conv.GetID() == jid.ToNonAD().String() {
				for _, histMsg := range conv.GetMessages() {
					parsedMsg, err := client.ParseWebMessage(jid, histMsg.GetMessage())
					if err != nil {
						continue
					}
					messages = append(messages, parsedMsg)
				}
				break
			}
		}

		var msgList []Message
		for idx, msg := range messages {
			t := msg.Info.Timestamp.Format("2006-01-02 15:04")
			msgType := "Other"
			var display string

			if conv := msg.Message.GetConversation(); conv != "" {
				msgType = "Text"
				display = conv
				if len(display) > 80 {
					display = display[:80] + "..."
				}
			} else if msg.Message.GetImageMessage() != nil {
				msgType = "Image"
				display = "Image message"
			} else if msg.Message.GetVideoMessage() != nil {
				msgType = "Video"
				display = "Video message"
			} else if msg.Message.GetDocumentMessage() != nil {
				msgType = "Doc"
				display = "Document message"
			} else {
				display = "Unknown message type"
			}

			sender := msg.Info.Sender.User
			if sender == "" {
				sender = "You"
			}

			msgList = append(msgList, Message{
				Index:   idx,
				Display: display,
				FullMsg: msg,
				Time:    t,
				Type:    msgType,
				Sender:  sender,
			})
		}

		return msgList, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for history sync response")
	}
}

func (c *Client) SendMessage(ctx context.Context, jid, message string) error {
	deviceStore, err := c.container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", false)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	if client.Store.ID == nil {
		return fmt.Errorf("client not logged in")
	}

	err = client.Connect()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect()

	recipient := types.NewJID(jid, "s.whatsapp.net")
	msg := message
	_, err = client.SendMessage(ctx, recipient, &waE2E.Message{
		Conversation: &msg,
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

func (c *Client) Close() {
	c.db.Close()
}
