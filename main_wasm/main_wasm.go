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

var goBridge struct {
	displayQRCode js.Value
	dialWebSocket js.Value
}

var client *whatsmeow.Client

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		fmt.Println("Successfully connected!")
	case *events.QR:
		fmt.Printf("QR code received. Total codes: %d\n", len(v.Codes))
		if len(v.Codes) > 0 {
			displayFunc := goBridge.displayQRCode
			if !displayFunc.IsUndefined() {
				fmt.Println("Invoking JS function displayQRCode...")
				displayFunc.Invoke(v.Codes[0])
			} else {
				fmt.Println("Error: JavaScript function 'displayQRCode' not found.")
			}
		}
	case *events.LoggedOut:
		fmt.Println("Logged out!")
		js.Global().Call("alert", "You have been logged out.")
	case *events.PairSuccess:
		fmt.Printf("Successfully paired with %s (%s)\n", v.BusinessName, v.ID)
	}
}

func main() {
	jsBridge := js.Global()
	goBridge.displayQRCode = jsBridge.Get("displayQRCode")
	goBridge.dialWebSocket = jsBridge.Get("dialWebSocket")

	if !goBridge.dialWebSocket.Truthy() {
		panic("The 'dialWebSocket' function was not found in the JavaScript global scope.")
	}

	fmt.Println("WhatsMeow WASM module starting...")

	jsStoreContainer := jsstore.NewJSStore()

	device, err := jsStoreContainer.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	log := waLog.Stdout("Client", "DEBUG", true)

	client = whatsmeow.NewClient(device, log)

	client.SetWSDialer(whatsmeow.NewJSDialer())

	client.AddEventHandler(eventHandler)

	err = client.Connect()
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}

	fmt.Println("Connection initiated. Waiting for events...")

	select {}
}
