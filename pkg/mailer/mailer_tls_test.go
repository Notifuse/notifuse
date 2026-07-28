package mailer

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// generateTestCert creates a self-signed certificate valid for 127.0.0.1,
// returning it plus a pool that trusts it.
func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// startMockSMTPSServer runs a minimal SMTP session behind a TLS listener
// (implicit TLS): greeting, EHLO and QUIT — enough for an unauthenticated
// go-mail dial. Returns the port and a stop function.
func startMockSMTPSServer(t *testing.T, cert tls.Certificate) (int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := tlsListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Bound the lazy server-side handshake normally performed by the
		// first I/O, so a peer that never sends a ClientHello cannot block
		// stop() forever.
		if tlsConn, ok := conn.(*tls.Conn); ok {
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn.SetDeadline(time.Time{})
		}
		reader := bufio.NewReader(conn)
		conn.Write([]byte("220 mock ESMTP\r\n"))
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			upper := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				conn.Write([]byte("250-mock\r\n250 SIZE 10485760\r\n"))
			case strings.HasPrefix(upper, "QUIT"):
				conn.Write([]byte("221 Bye\r\n"))
				return
			default:
				conn.Write([]byte("250 OK\r\n"))
			}
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	return port, func() {
		tlsListener.Close()
		<-done
	}
}

func TestSMTPMailer_CreateSMTPClient_ImplicitTLSDial(t *testing.T) {
	cert, pool := generateTestCert(t)
	port, stop := startMockSMTPSServer(t, cert)
	defer stop()

	prev := implicitTLSPort
	implicitTLSPort = port
	defer func() { implicitTLSPort = prev }()

	config := &Config{
		SMTPHost:  "127.0.0.1",
		SMTPPort:  port,
		FromEmail: "noreply@example.com",
		FromName:  "Test",
		UseTLS:    true,
	}
	client, err := NewSMTPMailer(config).createSMTPClient()
	if err != nil {
		t.Fatalf("createSMTPClient: %v", err)
	}
	if err := client.SetTLSConfig(&tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}); err != nil {
		t.Fatalf("SetTLSConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A TLS-first dial succeeding against an SMTPS-only server proves the
	// client uses implicit TLS on this port; a STARTTLS client would wait for
	// a plaintext greeting and time out.
	if err := client.DialWithContext(ctx); err != nil {
		t.Fatalf("DialWithContext over implicit TLS: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSMTPMailer_CreateSMTPClient_TLSDisabledOnImplicitPort(t *testing.T) {
	// Explicitly disabling TLS keeps the plaintext policy even on port 465.
	config := &Config{
		SMTPHost:  "smtp.example.com",
		SMTPPort:  465,
		FromEmail: "noreply@example.com",
		FromName:  "Test",
		UseTLS:    false,
	}
	client, err := NewSMTPMailer(config).createSMTPClient()
	if err != nil {
		t.Fatalf("createSMTPClient: %v", err)
	}
	if got := client.TLSPolicy(); got != "NoTLS" {
		t.Errorf("TLSPolicy = %q, want NoTLS", got)
	}
}
