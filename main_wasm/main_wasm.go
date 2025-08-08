//go:build wasm

package main

import (
	"context"
	"fmt"
	"syscall/js"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/jsstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var client *whatsmeow.Client

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		fmt.Println("Successfully connected!")
	case *events.QR:
		fmt.Printf("QR code received. Total codes: %d\n", len(v.Codes))
		if len(v.Codes) > 0 {
			// Get the JS function we need to call from the global scope.
			displayFunc := js.Global().Get("displayQRCode")
			if !displayFunc.IsUndefined() {
				// Call the JS function with the first QR code string.
				fmt.Println("Invoking JS function displayQRCode...")
				displayFunc.Invoke(v.Codes[0])
			} else {
				fmt.Println("Error: JavaScript function 'displayQRCode' not found.")
			}
		}
	case *events.LoggedOut:
		fmt.Println("Logged out!")
		// You might want to display a message to the user here.
		js.Global().Call("alert", "You have been logged out.")
	case *events.PairSuccess:
		fmt.Printf("Successfully paired with %s (%s)\n", v.BusinessName, v.ID)
	}
}

func main() {

	fmt.Println("WhatsMeow WASM module starting...")

	// 1. Create the JS storage container
	jsStoreContainer := jsstore.NewJSStore()

	// 2. Get a device from the container (or create a new one)
	// In a real app, you'd have logic to load an existing JID.
	device, err := jsStoreContainer.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	// 3. Set up logging (optional but recommended)
	log := waLog.Stdout("Client", "DEBUG", true)

	// 4. Create the client
	client = whatsmeow.NewClient(device, log)

	client.SetWSDialer(whatsmeow.NewJSDialer())

	// 5. Register an event handler
	client.AddEventHandler(eventHandler)

	// 6. Connect to WhatsApp
	err = client.Connect()
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}

	fmt.Println("Connection initiated. Waiting for events...")

	// 7. Keep the Go program running
	// A channel select is a common way to block forever.
	select {}
}
