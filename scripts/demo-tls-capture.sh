#!/usr/bin/env bash
# 演示一遍: 抓包可看到密文 vs 明文, 直观点对比。
# 需安装 tcpdump + 跑 docker mysql 8.0
set -euo pipefail

# 0. 启动一个测试用的 MySQL (root/secret, 已加载挂载的证书)
if ! docker ps --format '{{.Names}}' | grep -q '^mysql-tls$'; then
    docker run -d --name mysql-tls \
        -e MYSQL_ROOT_PASSWORD=secret \
        -e MYSQL_DATABASE=shop \
        -e MYSQL_USER=appuser \
        -e MYSQL_PASSWORD=apppass \
        -v "$(cd "$(dirname "$0")/.." && pwd)/certs":/certs:ro \
        mysql:8.0 \
        --ssl-ca=/certs/ca.pem \
        --ssl-cert=/certs/server.pem \
        --ssl-key=/certs/server.key \
        --require-secure-transport=ON
fi

echo "==== 抓包 5 秒 ===="
sudo tcpdump -i any -A 'port 3306' -w /tmp/mysql.pcap &
PID=$!
sleep 1

echo "==== 用 demo 客户端连一次 ===="
cd "$(dirname "$0")/.."
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_USER=appuser
export MYSQL_PASSWORD=apppass
export MYSQL_DB=shop
export MODE=tls
export MYSQL_CA_PATH="$(pwd)/certs/ca.pem"
go run . -mode=tls || true

sleep 1
kill "$PID" 2>/dev/null || true

echo "==== 用 strings 在 pcap 里翻一翻 ===="
strings /tmp/mysql.pcap | head -80
echo "==== 期望: 看到 'mysql_native_password' / 'SELECT' / 明文账号 等 是没加密; ===="
echo "====       如果看不到, 或只看到 TLS 握手的 Application Data, 就是成功加密。===="
