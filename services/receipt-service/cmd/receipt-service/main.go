package main

import (
	"errors"
	"log"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("receipt-service runtime wiring is idle; set NEXUSIM_RECEIPT_SERVICE_MODE after postgres repository is implemented")
		return nil
	default:
		return errors.New("unsupported NEXUSIM_RECEIPT_SERVICE_MODE")
	}
}
