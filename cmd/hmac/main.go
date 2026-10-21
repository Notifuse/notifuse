package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run cmd/hmac/main.go <email> <signing_key>")
		fmt.Println("The signing key is SIGNING_KEY, or SECRET_KEY when SIGNING_KEY is not set.")
		os.Exit(1)
	}

	email := os.Args[1]
	signingKey := os.Args[2]

	result := emailHMAC(email, signingKey)

	fmt.Println()
	fmt.Printf("Root Email: %s\n", email)
	fmt.Printf("HMAC: %s\n", result)
	fmt.Println()
	fmt.Printf("Reset URL: /api/demo.reset?hmac=%s\n", result)
}

// emailHMAC is the hex HMAC-SHA256 of email under signingKey, as /api/demo.reset
// verifies it.
func emailHMAC(email, signingKey string) string {
	h := hmac.New(sha256.New, []byte(signingKey))
	h.Write([]byte(email))
	return fmt.Sprintf("%x", h.Sum(nil))
}
