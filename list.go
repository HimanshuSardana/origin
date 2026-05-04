package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
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

func runListMode(jidFlag string) error {
	dbLog := waLog.Stdout("Database", "ERROR", false)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:origin.db?_foreign_keys=on", dbLog)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	db, err := sql.Open("sqlite3", "file:origin.db?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	var selectedJID string
	if jidFlag != "" {
		selectedJID = jidFlag
	} else {
		selectedJID, err = listContactsFZF(db)
		if err != nil {
			return fmt.Errorf("list contacts: %w", err)
		}
	}
	if selectedJID == "" {
		return nil // User cancelled
	}

	// Connect client
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", false)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			}
		}
	} else {
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}
	defer client.Disconnect()

	// List messages
	msg, err := listMessagesFZF(client, selectedJID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}
	if msg == nil {
		return nil // User cancelled
	}

	// Process selected message
	if conv := msg.Message.GetConversation(); conv != "" {
		// Text message - copy to clipboard
		if err := copyToClipboard(conv); err != nil {
			return fmt.Errorf("copy to clipboard: %w", err)
		}
		fmt.Println("Message copied to clipboard")
	} else {
		// Media message - download
		var mediaName string
		var mediaData []byte

		if img := msg.Message.GetImageMessage(); img != nil {
			mediaName = "image"
			mediaData, err = client.Download(context.Background(), img)
		} else if vid := msg.Message.GetVideoMessage(); vid != nil {
			mediaName = "video"
			mediaData, err = client.Download(context.Background(), vid)
		} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
			mediaName = "document"
			mediaData, err = client.Download(context.Background(), doc)
		} else {
			fmt.Println("Unsupported media type")
			return nil
		}

		if err != nil {
			return fmt.Errorf("download media: %w", err)
		}

		fmt.Printf("Enter filename to save as (default: %s): ", mediaName)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		filename := scanner.Text()
		if filename == "" {
			filename = mediaName
		}

		if err := os.WriteFile(filename, mediaData, 0644); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
		fmt.Println("File saved as", filename)
	}

	return nil
}

func listContactsFZF(db *sql.DB) (string, error) {
	rows, err := db.Query("SELECT their_jid, full_name FROM whatsmeow_contacts WHERE full_name != '' ORDER BY full_name")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var contacts []string
	for rows.Next() {
		var jid, fullName string
		if err := rows.Scan(&jid, &fullName); err != nil {
			return "", err
		}
		contacts = append(contacts, fmt.Sprintf("%s\t%s", fullName, jid))
	}

	if len(contacts) == 0 {
		return "", fmt.Errorf("no contacts found")
	}

	fzf := exec.Command("fzf", "--delimiter=\t", "--with-nth=1", "--prompt=Select contact: ")
	stdin, err := fzf.StdinPipe()
	if err != nil {
		return "", err
	}

	go func() {
		defer stdin.Close()
		for _, c := range contacts {
			fmt.Fprintln(stdin, c)
		}
	}()

	output, err := fzf.Output()
	if err != nil {
		return "", nil // User cancelled
	}

	parts := strings.SplitN(string(output), "\t", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), nil
	}
	return "", nil
}

func listMessagesFZF(client *whatsmeow.Client, chatJID string) (*events.Message, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return nil, err
	}

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
					OnDemandMsgCount: proto.Int32(20),
				},
			},
		},
	}

	if _, err := client.SendPeerMessage(context.Background(), req); err != nil {
		return nil, err
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

		if len(messages) == 0 {
			return nil, nil
		}

		var lines []string
		for idx, msg := range messages {
			t := msg.Info.Timestamp.Format("2006-01-02 15:04")
			var display string
			if conv := msg.Message.GetConversation(); conv != "" {
				display = fmt.Sprintf("[Text] %s: %s", t, conv)
			} else if msg.Message.GetImageMessage() != nil {
				display = fmt.Sprintf("[Image] %s", t)
			} else if msg.Message.GetVideoMessage() != nil {
				display = fmt.Sprintf("[Video] %s", t)
			} else if msg.Message.GetDocumentMessage() != nil {
				display = fmt.Sprintf("[Doc] %s", t)
			} else {
				display = fmt.Sprintf("[Other] %s", t)
			}
			lines = append(lines, fmt.Sprintf("%d\t%s", idx, display))
		}

		fzf := exec.Command("fzf", "--delimiter=\t", "--with-nth=2", "--prompt=Select message: ")
		stdin, err := fzf.StdinPipe()
		if err != nil {
			return nil, err
		}

		go func() {
			defer stdin.Close()
			for _, line := range lines {
				fmt.Fprintln(stdin, line)
			}
		}()

		output, err := fzf.Output()
		if err != nil {
			return nil, nil // User cancelled
		}

		parts := strings.SplitN(string(output), "\t", 2)
		if len(parts) < 1 {
			return nil, nil
		}

		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, err
		}

		if idx >= 0 && idx < len(messages) {
			return messages[idx], nil
		}
		return nil, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for history sync")
	}
}

func copyToClipboard(text string) error {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, text)
	}()
	return cmd.Run()
}
