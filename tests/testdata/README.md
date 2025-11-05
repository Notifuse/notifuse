# Test Data Directory

This directory contains test fixtures and data used across the test suite.

## Structure

```
testdata/
└── certs/               # TLS certificates for testing
    ├── test_cert.pem    # Self-signed X.509 certificate
    ├── test_key.pem     # RSA private key
    ├── README.md        # Certificate generation guide
    └── TEST_USAGE.md    # Usage documentation
```

## Certificates

The `certs/` directory contains self-signed TLS certificates used for testing the SMTP relay server with STARTTLS support.

### Key Details

- **Subject**: CN=localhost, O=Notifuse Test
- **Validity**: 10 years from generation
- **Type**: Self-signed (testing only)
- **Key Size**: RSA 2048-bit

### Usage

These certificates are automatically loaded by the SMTP relay e2e tests:

```go
certPath := filepath.Join("..", "testdata", "certs", "test_cert.pem")
keyPath := filepath.Join("..", "testdata", "certs", "test_key.pem")
```

### Regeneration

If the certificates expire or need to be regenerated:

```bash
cd tests/testdata/certs
openssl req -x509 -newkey rsa:2048 -keyout test_key.pem \
  -out test_cert.pem -days 3650 -nodes \
  -subj "/C=US/ST=Test/L=Test/O=Notifuse Test/CN=localhost"
```

## Adding New Test Data

When adding new test fixtures:

1. Create appropriate subdirectories under `testdata/`
2. Use descriptive names for fixtures
3. Document the purpose and format of test data
4. Keep test data minimal and focused
5. Avoid sensitive or production data

## Security Note

⚠️ **Never use production data or secrets in test fixtures.**

All test data should be:
- Synthetic/generated
- Non-sensitive
- Safe to commit to version control
- Clearly marked as test data

## See Also

- [Integration Test Guide](../README.md)
- [TLS Certificate Documentation](certs/README.md)
- [SMTP Relay Testing](../../SMTP_RELAY_TESTING.md)

