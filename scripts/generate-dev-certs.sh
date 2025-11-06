#!/bin/bash

# Script to generate self-signed TLS certificates for local development
# Usage: ./scripts/generate-dev-certs.sh [domain]

set -e

# Default domain
DOMAIN="${1:-localapi.notifuse.com}"
OUTPUT_DIR="./dev-certs"

echo "🔐 Generating self-signed TLS certificates for local development"
echo "Domain: $DOMAIN"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Generate private key and certificate
openssl req -x509 -newkey rsa:2048 \
  -keyout "$OUTPUT_DIR/${DOMAIN}.key.pem" \
  -out "$OUTPUT_DIR/${DOMAIN}.cert.pem" \
  -days 365 -nodes \
  -subj "/C=US/ST=Development/L=Local/O=Notifuse Dev/CN=$DOMAIN" \
  -addext "subjectAltName=DNS:$DOMAIN,DNS:localhost,IP:127.0.0.1"

echo "✅ Certificates generated successfully!"
echo ""
echo "📁 Location: $OUTPUT_DIR/"
echo "   - Certificate: ${DOMAIN}.cert.pem"
echo "   - Private Key: ${DOMAIN}.key.pem"
echo ""

# Generate base64 encoded versions for environment variables
CERT_BASE64=$(cat "$OUTPUT_DIR/${DOMAIN}.cert.pem" | base64)
KEY_BASE64=$(cat "$OUTPUT_DIR/${DOMAIN}.key.pem" | base64)

# Save base64 versions to a file
cat > "$OUTPUT_DIR/.env.smtp-relay" <<EOF
# SMTP Relay TLS Configuration (Base64 Encoded)
# Generated for: $DOMAIN
# Valid for: 365 days from $(date +%Y-%m-%d)
#
# Add these to your .env file:

SMTP_RELAY_ENABLED=true
SMTP_RELAY_PORT=587
SMTP_RELAY_DOMAIN=$DOMAIN

# Base64 encoded certificate and key
SMTP_RELAY_TLS_CERT_BASE64="$CERT_BASE64"
SMTP_RELAY_TLS_KEY_BASE64="$KEY_BASE64"
EOF

echo "📝 Environment variables saved to: $OUTPUT_DIR/.env.smtp-relay"
echo ""
echo "🚀 Quick Setup:"
echo "   1. Copy the contents of $OUTPUT_DIR/.env.smtp-relay to your .env file"
echo "   2. Add '127.0.0.1 $DOMAIN' to your /etc/hosts file"
echo "   3. Run 'make dev' to start the server"
echo ""
echo "🧪 Test with:"
echo "   swaks --to test@example.com \\"
echo "     --from sender@example.com \\"
echo "     --server $DOMAIN:587 \\"
echo "     --tls \\"
echo "     --tls-verify \\"
echo "     --tls-ca-path $OUTPUT_DIR/${DOMAIN}.cert.pem \\"
echo "     --auth-user workspace_id \\"
echo "     --auth-password \"your-api-key\" \\"
echo "     --body '{\"notification\": {...}}'"
echo ""
echo "⚠️  Note: These are self-signed certificates for DEVELOPMENT ONLY"
echo "    Never use self-signed certificates in production!"
echo ""

