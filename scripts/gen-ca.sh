#!/usr/bin/env bash
# 生成自签 CA + MySQL 服务端证书, 用于本地演示 TLS 连接。
# 用法: ./scripts/gen-ca.sh [hostname]
#   默认 hostname=mysql.example.com
# 产物:
#   certs/ca.pem         自签 CA (客户端用来校验服务端)
#   certs/server.pem     服务端证书 (CN=hostname, 含 SAN)
#   certs/server.key     服务端私钥
#   certs/server-bundle.pem  key+cert 合并 (部分发行版需要)
set -euo pipefail

HOST="${1:-mysql.example.com}"
DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
mkdir -p "$DIR"
cd "$DIR"

echo "[1/4] 生成 CA 私钥与自签根证书..."
openssl genrsa -out ca.key 2048 2>/dev/null
openssl req -x509 -new -nodes -key ca.key -days 3650 \
    -subj "/CN=mysqltls-demo CA" \
    -out ca.pem 2>/dev/null

echo "[2/4] 生成服务端私钥与 CSR (CN=$HOST)..."
openssl genrsa -out server.key 2048 2>/dev/null
openssl req -new -key server.key \
    -subj "/CN=$HOST" \
    -out server.csr 2>/dev/null

echo "[3/4] 写入 v3 扩展 (SAN 固定本机常见地址)..."
cat > server.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
subjectAltName=@alt_names
[alt_names]
DNS.1 = $HOST
DNS.2 = localhost
IP.1  = 127.0.0.1
EOF

echo "[4/4] 用 CA 签发服务端证书..."
openssl x509 -req -in server.csr \
    -CA ca.pem -CAkey ca.key -CAcreateserial \
    -days 365 -sha256 -extfile server.ext \
    -out server.pem 2>/dev/null

cat server.pem server.key > server-bundle.pem

echo
echo "==== 产物列表 ===="
ls -l "$DIR"
echo
echo "==== MySQL my.cnf 启用方式 ===="
cat <<CNF
[mysqld]
ssl_ca   = $DIR/ca.pem
ssl_cert = $DIR/server.pem
ssl_key  = $DIR/server.key
require_secure_transport = ON
CNF

echo
echo "==== 客户端使用时 ===="
echo "  MYSQL_CA_PATH=$DIR/ca.pem"
