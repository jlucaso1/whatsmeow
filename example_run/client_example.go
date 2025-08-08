package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// We no longer need a separate eventHandler function, as we'll define it
// as a closure within main() to get access to the client instance.

func main() {
	// Set up logging. "DEBUG" level is crucial to see the sent stanzas.
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	clientLog := waLog.Stdout("Client", "DEBUG", true)

	// Set up the database and container
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// Get or create a device store
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	// Create a new client
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// --- START of MODIFICATIONS ---

	// Define the event handler as a closure to capture the `client` variable.
	eventHandler := func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			// Log the received message's content
			receivedText := v.Message.GetConversation()
			fmt.Printf("Received message from %s: '%s'\n", v.Info.SourceString(), receivedText)

			// Check if the message is the "send" command
			if receivedText == "send" {
				fmt.Println("-> Received 'send' command, preparing to send a response...")

				// The response message content
				responseMsg := &waProto.Message{
					Conversation: proto.String("Hello from Signal E2EE!"),
				}

				// Send the response. The `client.SendMessage` function will internally
				// build the correct node. Because our client logger is set to DEBUG,
				// the library will automatically log the sent stanza.
				_, err := client.SendMessage(context.Background(), v.Info.Chat, responseMsg)
				if err != nil {
					fmt.Printf("Error sending message: %v\n", err)
				} else {
					fmt.Println("--> Response message sent successfully!")
				}
			}
		}
	}

	// Add the event handler to the client
	client.AddEventHandler(eventHandler)

	// --- END of MODIFICATIONS ---

	// Connect to WhatsApp or get QR code for pairing
	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
				// now print the QR code to the terminal
				fmt.Println("Scan the QR code above with your WhatsApp app to log in.")
				// spawn the command to display the QR code
				cmd := exec.Command("qrencode", "-t", "ansiutf8")
				cmd.Stdin = strings.NewReader(evt.Code)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("Failed to display QR code: %v\n", err)
				}

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

	// Wait for Ctrl+C to disconnect
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
