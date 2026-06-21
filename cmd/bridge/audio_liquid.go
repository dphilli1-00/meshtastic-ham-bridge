//go:build liquid

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dphilli/meshtastic-ham-bridge/internal/ham"
	"github.com/dphilli/meshtastic-ham-bridge/internal/ham/liquid"
)

// runAudioTest runs the --test-audio loopback using the liquid-dsp OFDM modem.
func runAudioTest(ctx context.Context, txName, rxName, pttPath string) {
	payload := []byte("audio-loopback-test")

	// PTT: CM108 if available, else VOX.
	var ptt ham.PTT = ham.VOXPTT{}
	if cm, err := ham.NewCM108PTT(pttPath); err != nil {
		fmt.Printf("CM108 PTT not available (%v), using VOX\n", err)
	} else {
		fmt.Printf("CM108 PTT ready\n")
		ptt = cm
		defer cm.Close()
	}

	fmt.Printf("Opening TX audio device %q (liquid-dsp OFDM)...\n", txName)
	tx, err := liquid.NewAdapter(txName, "", ptt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TX liquid adapter: %v\n", err)
		os.Exit(1)
	}
	defer tx.Close()

	fmt.Printf("Opening RX audio device %q...\n", rxName)
	rx, err := liquid.NewAdapter("", rxName, ham.VOXPTT{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "RX liquid adapter: %v\n", err)
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

	fmt.Println("Sending frame every 3s. Ctrl-C to stop.")
	for {
		if err := tx.SendFrame(ctx, payload); err != nil {
			fmt.Fprintf(os.Stderr, "send: %v\n", err)
			return
		}
		fmt.Printf("TX: %q\n", payload)
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}
