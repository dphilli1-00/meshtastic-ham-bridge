//go:build !liquid

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
)

// runAudioTest runs the --test-audio loopback using the Bell 202 AFSK modem.
func runAudioTest(ctx context.Context, txName, rxName, pttPath string) {
	payload := []byte("audio-loopback-test")

	fmt.Printf("Opening TX audio device %q (Bell 202 AFSK)...\n", txName)
	tx, err := ham.NewAudioTX(txName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TX audio: %v\n", err)
		os.Exit(1)
	}
	defer tx.Close()

	ptt, pttErr := ham.NewCM108PTT(pttPath)
	if pttErr != nil {
		fmt.Printf("CM108 PTT not available (%v), using VOX\n", pttErr)
	} else {
		fmt.Printf("CM108 PTT ready\n")
		tx.SetPTT(ptt)
		defer ptt.Close()
	}

	fmt.Printf("Opening RX audio device %q...\n", rxName)
	rx, err := ham.NewAudioRX(rxName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "RX audio: %v\n", err)
		os.Exit(1)
	}
	defer rx.Close()

	go func() {
		for {
			f, err := rx.RecvFrame(ctx)
			if err != nil {
				return
			}
			if bytes.Contains(f, payload) {
				fmt.Printf("PASS — RX matched (%d bytes): % X\n", len(f), f)
			} else {
				fmt.Printf("RX (%d bytes, no match): % X\n", len(f), f)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-time.After(500 * time.Millisecond):
				fmt.Printf("  energy: %.6f\n", rx.Energy())
			case <-ctx.Done():
				return
			}
		}
	}()

	fmt.Println("Sending frame every 2s. Ctrl-C to stop.")
	for {
		if err := tx.SendFrame(ctx, payload); err != nil {
			fmt.Fprintf(os.Stderr, "send: %v\n", err)
			return
		}
		fmt.Printf("TX: %q\n", payload)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}
