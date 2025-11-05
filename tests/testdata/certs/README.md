# Test TLS Certificates

This directory contains self-signed TLS certificates for testing purposes only.

## Files

- `test_cert.pem`: Self-signed TLS certificate
- `test_key.pem`: Private key for the certificate

## Usage

These certificates are used in the SMTP relay end-to-end tests to enable TLS authentication without requiring real certificates.

**WARNING**: These are self-signed certificates and should NEVER be used in production. They are for testing purposes only.

## Regenerating Certificates

If you need to regenerate the certificates (e.g., if they expire), run:

```bash
cd testdata/certs
openssl req -x509 -newkey rsa:2048 -keyout test_key.pem -out test_cert.pem -days 3650 -nodes \
  -subj "/C=US/ST=Test/L=Test/O=Notifuse Test/CN=localhost"
```

The certificates are valid for 10 years from the generation date.

## Security Note

These certificates use SHA-256 with RSA encryption and are sufficient for local testing. The `-nodes` flag means the private key is not encrypted with a passphrase, which is appropriate for automated testing but should never be done with production certificates.
