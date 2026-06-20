//go:build windows

package mesh

import (
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

//go:embed ble_pair_page.html
var blePairPageHTML []byte

// webPairBLE opens a local browser page that uses the Web Bluetooth API to
// pair the device. Chrome/Edge store the bond in the Windows BLE stack, so
// after this returns our tinygo connection can use the bonded device.
//
// Blocks until the page signals /done (user completed pairing) or timeout.
func webPairBLE(macAddr string) error {
	done := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(blePairPageHTML) //nolint:errcheck
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("BLE web pair: listen: %w", err)
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	pairURL := fmt.Sprintf("http://127.0.0.1:%d/?mac=%s", port, url.QueryEscape(macAddr))

	fmt.Printf("BLE: opening pairing page at %s\n", pairURL)
	if !openInChromiumBrowser(pairURL) {
		fmt.Printf("BLE: could not find Chrome or Edge — open manually in Chrome/Edge: %s\n", pairURL)
	}

	select {
	case <-done:
		fmt.Println("BLE: web pairing complete.")
		return nil
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("BLE: timed out waiting for web pairing")
	}
}

// openInChromiumBrowser tries Chrome then Edge (both support Web Bluetooth).
// Returns true if a browser was launched successfully.
func openInChromiumBrowser(targetURL string) bool {
	// Common install paths for Chrome and Edge on Windows.
	candidates := []string{
		os.Getenv("PROGRAMFILES") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("PROGRAMFILES(X86)") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
		os.Getenv("PROGRAMFILES") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("PROGRAMFILES(X86)") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("LOCALAPPDATA") + `\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			if err := exec.Command(path, targetURL).Start(); err == nil {
				return true
			}
		}
	}
	return false
}
