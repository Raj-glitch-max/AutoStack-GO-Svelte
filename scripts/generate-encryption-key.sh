#!/bin/bash
# Generate encryption key for AutoStack
# This script creates a .env file with a secure encryption key if it doesn't exist

set -e

ENV_FILE=".env"

if [ -f "$ENV_FILE" ]; then
    # Check if key already exists
    if grep -q "^AUTOSTACK_ENCRYPTION_KEY=" "$ENV_FILE"; then
        echo "✓ Encryption key already exists in $ENV_FILE"
        echo "⚠️  WARNING: Never change this key after deployment - all encrypted data will become unreadable"
        exit 0
    fi
fi

echo "Generating new encryption key..."
KEY=$(openssl rand -base64 32)

if [ ! -f "$ENV_FILE" ]; then
    # Create new .env from example
    if [ -f ".env.example" ]; then
        cp .env.example "$ENV_FILE"
        echo "Created $ENV_FILE from .env.example"
    else
        touch "$ENV_FILE"
        echo "Created new $ENV_FILE"
    fi
fi

# Add or update the encryption key
if grep -q "^AUTOSTACK_ENCRYPTION_KEY=" "$ENV_FILE"; then
    # Key exists but is empty, update it
    sed -i.bak "s|^AUTOSTACK_ENCRYPTION_KEY=.*|AUTOSTACK_ENCRYPTION_KEY=$KEY|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
else
    # Add key
    echo "AUTOSTACK_ENCRYPTION_KEY=$KEY" >> "$ENV_FILE"
fi

echo "✓ Encryption key generated and saved to $ENV_FILE"
echo "⚠️  IMPORTANT: Back up this key securely. If lost, all encrypted AWS credentials will be unrecoverable."
echo ""
echo "Key: $KEY"
