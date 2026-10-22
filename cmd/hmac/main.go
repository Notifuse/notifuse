package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

// run prints the HMAC /api/demo.reset verifies for args[0] under args[1], the
// signing key, and returns the exit code.
func run(args []string, out io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(out, "Usage: go run cmd/hmac/main.go <email> <signing_key>")
		fmt.Fprintln(out, "The signing key is SIGNING_KEY, or SECRET_KEY when SIGNING_KEY is not set.")
		return 1
	}

	email := args[0]
	signingKey := args[1]

	// What domain.ComputeEmailHMAC computes, without importing the domain package
	// and its dependencies into a one-shot tool.
	result := crypto.ComputeHMAC256([]byte(email), signingKey)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Root Email: %s\n", email)
	fmt.Fprintf(out, "HMAC: %s\n", result)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Reset URL: /api/demo.reset?hmac=%s\n", result)
	return 0
}
