#!/bin/bash
set -e

CERT_DIR="../k8s/base/core/database/postgres/ssl-certs"
mkdir -p $CERT_DIR

# CA秘密鍵生成
openssl genrsa -out $CERT_DIR/ca.key 4096

# CA証明書生成
openssl req -new -x509 -days 3650 -key $CERT_DIR/ca.key -out $CERT_DIR/ca.crt \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Project/OU=Database/CN=Alt-CA"

# サーバー秘密鍵生成
openssl genrsa -out $CERT_DIR/server.key 4096

# サーバー証明書署名要求生成
openssl req -new -key $CERT_DIR/server.key -out $CERT_DIR/server.csr \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Project/OU=Database/CN=postgres"

# サーバー証明書生成 (SAN設定含む)
cat > $CERT_DIR/server.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req

[req_distinguished_name]

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = postgres
DNS.2 = postgres.alt-database.svc.cluster.local
DNS.3 = db.alt-database.svc.cluster.local
DNS.4 = localhost
IP.1 = 127.0.0.1
EOF

openssl x509 -req -days 365 -in $CERT_DIR/server.csr -CA $CERT_DIR/ca.crt \
  -CAkey $CERT_DIR/ca.key -CAcreateserial -out $CERT_DIR/server.crt \
  -extensions v3_req -extfile $CERT_DIR/server.conf

# 権限設定
chmod 600 $CERT_DIR/*.key
chmod 644 $CERT_DIR/*.crt

echo "SSL certificates generated successfully in $CERT_DIR"

# generate auth-postgres-ssl-secret
AUTH_POSTGRES_SSL_DIR="../k8s/base/core/database/auth-postgres/ssl-certs"
mkdir -p $AUTH_POSTGRES_SSL_DIR

openssl genrsa -out $AUTH_POSTGRES_SSL_DIR/ca.key 4096

openssl req -new -x509 -days 365 -key $AUTH_POSTGRES_SSL_DIR/ca.key -out $AUTH_POSTGRES_SSL_DIR/ca.crt \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Project/OU=Database/CN=Alt-CA"

openssl genrsa -out $AUTH_POSTGRES_SSL_DIR/server.key 4096

openssl req -new -key $AUTH_POSTGRES_SSL_DIR/server.key -out $AUTH_POSTGRES_SSL_DIR/server.csr \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Project/OU=Database/CN=postgres"


# auth-postgres-ssl-secret
cat > $AUTH_POSTGRES_SSL_DIR/server.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req

[req_distinguished_name]

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = auth-postgres
DNS.2 = auth-postgres.alt-auth.svc.cluster.local
DNS.3 = auth-postgres.alt-auth.svc.cluster.local
DNS.4 = localhost
IP.1 = 127.0.0.1
EOF

openssl x509 -req -days 365 -in $AUTH_POSTGRES_SSL_DIR/server.csr -CA $AUTH_POSTGRES_SSL_DIR/ca.crt \
  -CAkey $AUTH_POSTGRES_SSL_DIR/ca.key -CAcreateserial -out $AUTH_POSTGRES_SSL_DIR/server.crt \
  -extensions v3_req -extfile $AUTH_POSTGRES_SSL_DIR/server.conf

chmod 600 $AUTH_POSTGRES_SSL_DIR/*.key
chmod 644 $AUTH_POSTGRES_SSL_DIR/*.crt

echo "SSL certificates generated successfully in $AUTH_POSTGRES_SSL_DIR"