package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/travel-expense-approval/internal/domain"
	"example.com/travel-expense-approval/internal/protocol"
)

func main() {
	stateDir := flag.String("state", "", "data directory for persisted state")
	flag.Parse()

	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "missing required --state directory")
		os.Exit(2)
	}

	svc, err := domain.Open(*stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to open state:", err)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := protocol.ProcessLine(svc, line)
		if err := encoder.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "failed to write response:", err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "failed to read input:", err)
		os.Exit(1)
	}
}
