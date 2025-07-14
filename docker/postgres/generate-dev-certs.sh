#!/bin/bash
set -e

CERT_DIR="./docker/postgres/ssl"
mkdir -p $CERT_DIR

# CA秘密鍵生成
openssl genrsa -out $CERT_DIR/ca.key 2048

# CA証明書生成
openssl req -new -x509 -days 365 -key $CERT_DIR/ca.key -out $CERT_DIR/ca.crt \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Dev/OU=Database/CN=Alt-Dev-CA"

# サーバー秘密鍵生成
openssl genrsa -out $CERT_DIR/server.key 2048

# サーバー証明書署名要求生成
openssl req -new -key $CERT_DIR/server.key -out $CERT_DIR/server.csr \
  -subj "/C=JP/ST=Tokyo/L=Tokyo/O=Alt-Dev/OU=Database/CN=db"

# サーバー証明書生成
openssl x509 -req -days 365 -in $CERT_DIR/server.csr -CA $CERT_DIR/ca.crt \
  -CAkey $CERT_DIR/ca.key -CAcreateserial -out $CERT_DIR/server.crt

# 権限設定（PostgreSQLが読める権限）
chmod 600 $CERT_DIR/server.key $CERT_DIR/ca.key
chmod 644 $CERT_DIR/server.crt $CERT_DIR/ca.crt

# PostgreSQL用のファイル所有者設定は不要（Dockerで999:999に自動設定）

echo "Development SSL certificates generated successfully"