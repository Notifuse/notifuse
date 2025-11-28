package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run cmd/hmac/main.go <email> <secret_key>")
		os.Exit(1)
	}

	email := os.Args[1]
	secretKey := os.Args[2]

	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(email))
	result := fmt.Sprintf("%x", h.Sum(nil))

	fmt.Println()
	fmt.Printf("Root Email: %s\n", email)
	fmt.Printf("HMAC: %s\n", result)
	fmt.Println()
	fmt.Printf("Reset URL: /api/demo.reset?hmac=%s\n", result)
}
