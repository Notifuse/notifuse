package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run prints the HMAC /api/demo.reset verifies for args[0] under args[1], the
// SECRET_KEY value, and returns the exit code. A key the server refuses is reported on
// errOut.
func run(args []string, out, errOut io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(out, "Usage: go run cmd/hmac/main.go <email> <secret_key>")
		return 1
	}

	email := args[0]
	// A SECRET_KEY listing several keys signs with its first one.
	security, err := config.NewSecurityConfig(args[1], "SECRET_KEY")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}

	result := domain.ComputeEmailHMAC(email, security.SecretKey)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Root Email: %s\n", email)
	fmt.Fprintf(out, "HMAC: %s\n", result)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Reset URL: /api/demo.reset?hmac=%s\n", result)
	return 0
}
