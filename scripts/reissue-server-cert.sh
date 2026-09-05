#!/usr/bin/env bash
# 用「现有 CA」(certs/ca.pem + ca.key) 为新的访问地址(IP/DNS)重新签发 MySQL 服务端证书。
# CA 不变 → 客户端里 go:embed 的 ca.pem 无需重新编译, 依然能验签新证书。
#
# 适用场景: 服务器已部署本仓库证书, 但连接地址不在证书 SAN 里, 客户端报
#   x509: cannot validate certificate for <ip> because it doesn't contain any IP SANs
#
# 用法:
#   ./scripts/reissue-server-cert.sh 103.42.30.173              # 为公网 IP 签发
#   ./scripts/reissue-server-cert.sh db.example.com 10.0.0.5    # DNS + IP 混合
# 产物: 覆盖 certs/server.pem 与 certs/server-bundle.pem (server.key 不变)
# 部署: 把新 server.pem/server-bundle.pem 拷到 MySQL 服务器替换, 再重启 mysqld。
set -euo pipefail

if [ "$#" -lt 1 ]; then
    echo "用法: $0 <host-or-ip> [host-or-ip ...]" >&2
    exit 1
fi

DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
cd "$DIR"

[ -f ca.pem ] && [ -f ca.key ] || { echo "缺 certs/ca.pem 或 ca.key, 先跑 ./scripts/gen-ca.sh" >&2; exit 1; }
[ -f server.key ] || { echo "缺 certs/server.key" >&2; exit 1; }

HOSTS=("$@")
CN="${HOSTS[0]}"

# 备份旧证书, 方便回滚
[ -f server.pem ] && cp server.pem server.pem.bak
[ -f server-bundle.pem ] && cp server-bundle.pem server-bundle.pem.bak

# 生成 v3 扩展: 自动把 IP 填进 IP.x、域名填进 DNS.x
EXT="$DIR/.server.reissue.ext"
{
    echo "authorityKeyIdentifier=keyid,issuer"
    echo "basicConstraints=CA:FALSE"
    echo "subjectAltName=@alt_names"
    echo "[alt_names]"
} > "$EXT"

d=0; ip=0
add_host() { # $1=value; 是 IPv4 则进 IP.x, 否则进 DNS.x
    if [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        ip=$((ip+1)); echo "IP.$ip = $1" >> "$EXT"
    else
        d=$((d+1));  echo "DNS.$d = $1" >> "$EXT"
    fi
}
for h in "${HOSTS[@]}"; do add_host "$h"; done
# 保留回环与本机, 兼容本地 docker(127.0.0.1) 和 localhost
add_host "localhost"
add_host "127.0.0.1"
add_host "0.0.0.0"
add_host "103.42.30.173"

echo "[1/3] 用现有 CA 重签服务端证书 (CN=$CN, SAN 见下)"
openssl req -new -key server.key -subj "/CN=$CN" -out server.csr
openssl x509 -req -in server.csr \
    -CA ca.pem -CAkey ca.key -CAcreateserial \
    -days 365 -sha256 -extfile "$EXT" \
    -out server.pem
cat server.pem server.key > server-bundle.pem

echo "[2/3] 新证书 SAN:"
openssl x509 -in server.pem -noout -ext subjectAltName

echo "[3/3] 完成。旧证书备份为 certs/server.pem.bak / server-bundle.pem.bak"
echo "部署: 将新 server.pem 与 server-bundle.pem 覆盖到 MySQL 服务器, 然后"
echo "      systemctl restart mysqld  或(容器) docker compose restart mysql"
