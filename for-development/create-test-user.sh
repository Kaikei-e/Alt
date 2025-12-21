#!/bin/bash

# Create a test user in Kratos
# Email: test-user@example.com
# Password: password123

echo "Creating test user..."

curl -X POST http://localhost:4434/admin/identities \
  -H "Content-Type: application/json" \
  -d '{
    "schema_id": "default",
    "traits": {
      "email": "test-user@example.com",
      "name": {
        "first": "Test",
        "last": "User"
      }
    },
    "credentials": {
      "password": {
        "config": {
          "password": "password123"
        }
      }
    }
  }'

echo ""
echo "---------------------------------------------------"
echo "Test user creation attempt finished."
echo "Credentials:"
echo "  Email:    test-user@example.com"
echo "  Password: password123"
echo "---------------------------------------------------"
