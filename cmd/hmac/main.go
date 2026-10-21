package main

import (
	"fmt"
	"os"

	"github.com/Notifuse/notifuse/internal/domain"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run cmd/hmac/main.go <email> <signing_key>")
		fmt.Println("The signing key is SIGNING_KEY, or SECRET_KEY when SIGNING_KEY is not set.")
		os.Exit(1)
	}

	email := os.Args[1]
	signingKey := os.Args[2]

	// The same HMAC /api/demo.reset verifies.
	result := domain.ComputeEmailHMAC(email, signingKey)

	fmt.Println()
	fmt.Printf("Root Email: %s\n", email)
	fmt.Printf("HMAC: %s\n", result)
	fmt.Println()
	fmt.Printf("Reset URL: /api/demo.reset?hmac=%s\n", result)
}
