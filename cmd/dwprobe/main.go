package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	path := os.Getenv("DIREWOLF_PATH")
	if path == "" {
		fmt.Println("set DIREWOLF_PATH")
		os.Exit(1)
	}

	cmd := exec.Command(path, "-t", "0", "-c", "NUL")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	fmt.Println("starting direwolf...")
	if err := cmd.Start(); err != nil {
		fmt.Printf("start error: %v\n", err)
		os.Exit(1)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		fmt.Printf("exited: %v\n", err)
	case <-time.After(5 * time.Second):
		fmt.Println("killing after 5s")
		cmd.Process.Kill()
		<-done
	}

	fmt.Printf("output len: %d\n", buf.Len())
	fmt.Printf("output:\n%s\n", buf.String())
}
