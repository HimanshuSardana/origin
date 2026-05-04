package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	qrterminal "github.com/mdp/qrterminal"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func displayQRCode(code string) {
	config := qrterminal.Config{
		Level:      qrterminal.M,
		Writer:     os.Stdout,
		HalfBlocks: true,
	}
	qrterminal.GenerateWithConfig(code, config)
	fmt.Println("\nScan this QR code with WhatsApp on your phone")
	fmt.Println("Or press Ctrl+C to exit")
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println("Received a message!", v.Message.GetConversation())
	}
}

func listContacts(db *sql.DB) (string, error) {
	rows, err := db.Query("SELECT their_jid, full_name FROM whatsmeow_contacts WHERE full_name != '' ORDER BY full_name")
	if err != nil {
		return "", fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	var contactList []Contact
	for rows.Next() {
		var jid, fullName string
		if err := rows.Scan(&jid, &fullName); err != nil {
			return "", fmt.Errorf("scan row: %w", err)
		}
		contactList = append(contactList, Contact{
			JID:     jid,
			Name:    fullName,
			Preview: "Click to view messages...",
		})
	}

	if len(contactList) == 0 {
		fmt.Println("No contacts found")
		return "", nil
	}

	var selectedContact Contact
	showContactPicker(contactList, func(c Contact) {
		selectedContact = c
	})

	return selectedContact.JID, nil
}

func listMessages(client *whatsmeow.Client, chatJID string) (*events.Message, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return nil, fmt.Errorf("parse JID: %w", err)
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
					OnDemandMsgCount: proto.Int32(10),
				},
			},
		},
	}
	_, err = client.SendPeerMessage(context.Background(), req)
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

		if len(messages) == 0 {
			return nil, nil
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

		var selectedMsg Message
		showMessagePicker(msgList, func(m Message) {
			selectedMsg = m
		})

		if selectedMsg.FullMsg == nil {
			return nil, nil
		}

		return selectedMsg.FullMsg, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for history sync response")
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

func run(listFlag bool, jidFlag string) {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:origin.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	if listFlag {
		// Open direct DB connection for contact listing
		db, err := sql.Open("sqlite3", "file:origin.db?_foreign_keys=on")
		if err != nil {
			panic(err)
		}
		defer db.Close()

		var selectedJID string
		if jidFlag != "" {
			selectedJID = jidFlag
		} else {
			selectedJID, err = listContacts(db)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing contacts: %v\n", err)
				os.Exit(1)
			}
		}
		if selectedJID == "" {
			return
		}

		// Connect client for media downloads
		deviceStore, err := container.GetFirstDevice(ctx)
		if err != nil {
			panic(err)
		}
		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)
		if client.Store.ID == nil {
			qrChan, _ := client.GetQRChannel(context.Background())
			err = client.Connect()
			if err != nil {
				panic(err)
			}
			for evt := range qrChan {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				} else {
					fmt.Println("Login event:", evt.Event)
				}
			}
		} else {
			err = client.Connect()
			if err != nil {
				panic(err)
			}
		}
		defer client.Disconnect()

		// List messages
		msg, err := listMessages(client, selectedJID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing messages: %v\n", err)
			os.Exit(1)
		}
		if msg == nil {
			return
		}

		// Process message
		if conv := msg.Message.GetConversation(); conv != "" {
			if err := copyToClipboard(conv); err != nil {
				fmt.Fprintf(os.Stderr, "Error copying to clipboard: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Message copied to clipboard")
		} else {
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
				return
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error downloading media: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Enter filename to save as (default: %s): ", mediaName)
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			filename := scanner.Text()
			if filename == "" {
				filename = mediaName
			}
			if err := os.WriteFile(filename, mediaData, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving file: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("File saved as", filename)
		}
		return
	}

	// Normal flow - If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		fmt.Println("Already logged in, connected to WhatsApp Web server")
		// prompt for phoneNumber
		phoneNumber := "919899004405"
		recipient := types.NewJID(phoneNumber, "s.whatsapp.net")
		message := "Hello from WhatsMeow!"
		_, err := client.SendMessage(context.Background(), recipient, &waE2E.Message{
			Conversation: &message,
		})
		if err != nil {
			fmt.Println("Error sending message:", err)
		}

	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
