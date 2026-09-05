# mysqltls-demo

Go 连接 MySQL 防网络抓包的最小可运行示例，覆盖三种主流方案：

| MODE | 加密层 | 验证方式 | 适用场景 |
|---|---|---|---|
| `tls` | MySQL 原生 TLS | CA 验签 + 主机名校验 | **生产首选** |
| `pin` | MySQL 原生 TLS | 自比对服务端 SPKI 指纹 | 内部 CA 不便分发 / 防 CA 被钓 |
| `ssh` | SSH 加密隧道 | SSH 主机密钥 (known_hosts) | 不改 MySQL 配置也能加密 |

## 目录结构

```
mysqltls-demo/
├── main.go              入口与命令行参数
├── config.go            配置加载 (全部来自环境变量, 0 明文凭证入仓库)
├── tls.go               TLS 预设注册 (verify-full / pin / skip-pinning)
├── tunnel.go            SSH 拨号器注册 (自定义协议 "ssh")
├── db.go                连接池与查询演示, 含密码掩码
├── certs_embed.go       用 go:embed 把 certs/ca.pem 打包进二进制 (CA 路径变可选)
├── docker-compose.yml   Docker 起 TLS MySQL (mysql-tls)
├── deploy/my.cnf        MySQL 服务端 TLS 配置 (挂载进容器 conf.d)
├── cmd/probe-server-cert/  打印 MySQL 实际出示的证书链 (排查 x509 SAN 报错)
├── scripts/
│   ├── gen-ca.sh        一键生成自签 CA + 服务端证书
│   ├── reissue-server-cert.sh  用现有 CA 为指定 IP/DNS 重签服务端证书 (CA 不变)
│   └── demo-tls-capture.sh   抓包直观看明文 vs 密文
├── .env.example
├── go.mod / go.sum
└── README.md
```

## 〇、Docker 一键起一个 TLS MySQL（本地跑 demo 最快）

```bash
# 1. 起库 (首次会拉 mysql:8.0 镜像并初始化数据卷, 耐心等它 healthy)
docker compose up -d

# 2. 等 MySQL 就绪 (回到 shell 出现 done 字样即可, 或等 healthcheck 通过)
docker compose ps

# 3. 连 127.0.0.1:3306。CA 已内嵌, 不需要 MYSQL_CA_PATH
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_USER=appuser
export MYSQL_PASSWORD=apppass
export MYSQL_DB=shop
export MODE=tls
go run .
```

- 容器把 `certs/server.pem` / `certs/server.key` 以 **只读** 方式挂进 mysqld,
  并用 `--require-secure-transport=ON` 全局强制 TLS。
- 连接地址用 `127.0.0.1` 是因为自签证书 SAN 里带 `IP:127.0.0.1`/`DNS:localhost`;
  想用别的主机名, 先 `./scripts/gen-ca.sh <hostname>` 重签再 `docker compose up -d --force-recreate`。
- 默认密码只是本地 demo 值; 可用环境变量覆盖:
  `MYSQL_ROOT_PASSWORD` / `MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_DATABASE` / `MYSQL_PORT`。
  数据卷只在**首次**初始化时读取这些值, 之后再改密码需重建卷:
  `docker compose down -v && docker compose up -d`。
- 停库: `docker compose down` (加 `-v` 连数据卷一起删)。

## 〇、已验证的真实环境

| 项 | 值 |
|---|---|
| 远程 MySQL | `103.42.30.173:17941` (docker 8.0.46) |
| 端到端密文 | `TLS_AES_128_GCM_SHA256` |
| CA 证书 | `certs/ca.pem` (服务器已就位, 已拉回本地) |
| 服务端证书 SAN | `IP:103.42.30.173, DNS:localhost, IP:127.0.0.1` |
| `require_secure_transport` | `ON` (明文连接会被拒绝并报 `ERROR 3159`) |

复现命令：

```bash
cd /Users/uuxia/WorkBuddy/2026-09-05-12-12-05/mysqltls-demo
MODE=tls \
MYSQL_HOST=103.42.30.173 MYSQL_PORT=17941 \
MYSQL_USER=root MYSQL_PASSWORD='<凭据>' \
MYSQL_DB=test001 \
MYSQL_CA_PATH="$PWD/certs/ca.pem" \
go run . -mode=tls
```

成功标志：
```
[dial] root:***@tcp(103.42.30.173:17941)/test001?tls=verify-full&...
[ssl]  cipher = "TLS_AES_128_GCM_SHA256"
[sql]  SELECT 1 = 1
OK - connection is up & query works
```

## 一、跑起来 (TLS + CA 方案)

```bash
# 1. 装依赖
go mod tidy

# 2. 生成自签 CA + 服务端证书 (默认 hostname=mysql.example.com)
./scripts/gen-ca.sh

# 3. 让 MySQL 用上证书
#    把脚本末尾打印的 4 行 (ssl_ca/ssl_cert/ssl_key + require_secure_transport=ON)
#    写到 my.cnf 的 [mysqld] 节, 并 `systemctl restart mysqld`

# 4. 设置环境变量并跑
#    CA 已经用 go:embed 打进二进制, MYSQL_CA_PATH 可省略。
#    只有当你要临时覆盖 CA (如轮换期) 时才 export MYSQL_CA_PATH。
export MYSQL_HOST=mysql.example.com
export MYSQL_PORT=3306
export MYSQL_USER=appuser
export MYSQL_PASSWORD=secret
export MYSQL_DB=shop
export MODE=tls

go run . -mode=tls
# 期望输出:
#   [dial] appuser:***@tcp(mysql.example.com:3306)/shop?tls=verify-full&...
#   [ssl]  cipher = "TLS_AES_256_GCM_SHA384"
#   [sql]  SELECT 1 = 1
#   OK - connection is up & query works
```

`[ssl] cipher = "..."` 非空就是真的 TLS，空字符串则提示未加密。

> **CA 打包说明**: `certs_embed.go` 用 `go:embed` 把 `certs/ca.pem` 编进二进制,
> 所以上面不设 `MYSQL_CA_PATH` 也能跑, 部署只带一个可执行文件即可。
> 换 CA / 续期后需要重新 `go build`; 若想"换 CA 不重编", 才临时
> `export MYSQL_CA_PATH=/path/to/new-ca.pem` 覆盖。私钥 (`ca.key`/`server.key`)
> **永不进二进制**, 也请勿自行改动 embed 范围把它们加进去。

## 二、公钥固定 (pin) 模式

```bash
# 1. 拿到服务端证书的 SPKI 指纹 (base64)
openssl x509 -in certs/server.pem -pubkey -noout | \
    openssl pkey -pubin -outform DER | \
    openssl dgst -sha256 -binary | base64

# 2. 设置 (CA 已内嵌, MYSQL_CA_PATH 无需设置)
export MODE=pin
export MYSQL_SPKI_FP="<上面那串 base64>"

go run . -mode=pin
```

后续服务端换证书 / 续期 *只要私钥不换*，指纹不变，业务不需要改 config。

## 三、SSH 隧道 (不改 MySQL)

```bash
export MODE=ssh
# Host 字段是 MySQL 服务端的真实地址, 客户端先连跳板机再到它
export MYSQL_HOST=mysql.internal
export MYSQL_PORT=3306
export MYSQL_USER=appuser
export MYSQL_PASSWORD=secret
export MYSQL_DB=shop
export MYSQL_SSH_HOST=jump.example.com:22
export MYSQL_SSH_USER=deploy
export MYSQL_SSH_KEY_PATH="$HOME/.ssh/jump_key"

go run . -mode=ssh
```

DSN 自动变成 `appuser:***@ssh(mysql.internal:3306)/shop?tls=skip-pinning`，driver 调
用 `mysql.RegisterDialContext("ssh", ...)` 注册的回调走 SSH 隧道。

> 等价 shell 行: `ssh -L 13306:mysql.internal:3306 -i jump_key deploy@jump.example.com -N`
> 然后用 `tcp(127.0.0.1:13306)/shop`。两者本质上都是 SSH 加密隧道，区别在端口托管者。

## 四、抓包验证

```bash
./scripts/demo-tls-capture.sh
```

会启动 Docker MySQL 8.0、运行 demo 客户端、tcpdump 抓 3306 端口、最后把 `strings` 结果输出。
TLS 模式下应该看不到 `SELECT`、账号密码等明文；如果是用未加密的默认配置，则能直接读出来。

## 五、关键安全建议

- 用 prepared statement (`db.Query/Exec` 配合 `?`) 而不要拼接 SQL，避免注入。
- 凭据一律 `os.Getenv`，不要写进仓库。CI 用 secret manager 注入。
- 业务账号给最小权限：`CREATE USER 'app'@'10.0.1.%' IDENTIFIED BY '...' REQUIRE SSL;`
  `GRANT SELECT, INSERT, UPDATE, DELETE ON shop.* TO 'app'@'10.0.1.%';`
- MySQL 8 默认走 `caching_sha2_password`，握手阶段就要小心 — 把 TLS 先建立起来，再走 auth。
  我们的 demo `tls=verify-full|skip-pinning` 都已经覆盖这一点。
- 不要把 `InsecureSkipVerify: true` 与生产连接同时使用（本 demo 仅 ssh 模式用到）。

## 六、依赖

- `github.com/go-sql-driver/mysql v1.8.1`
- `golang.org/x/crypto v0.27.0` (只用 ssh/knownhosts)

不依赖 `sqlx`、`gorm`，保持最小化。实际项目可在这层之上封装 repository。

## 七、常见报错

先用探测工具看服务器到底出示什么证书(在能连通 MySQL 的机器上跑):

```bash
go run ./cmd/probe-server-cert 103.42.30.173:3306
```

**`cert[0]` 的 subject 是 `CN=MySQL_Server_..._Auto_Generated_...`(自动生成证书)**

原因: 服务端的自定义 TLS 配置(`ssl_cert`/`ssl_key`/`ssl_ca`)没有生效,
MySQL 8 在没配 `ssl_cert` 时会在数据目录自动生成一套证书撑 TLS。

排查与修复(在 MySQL 服务器那台上):
```bash
# 1. 看 mysqld 实际生效的 TLS 变量
docker compose exec mysql sh -c \
  'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N \
   -e "SHOW VARIABLES WHERE Variable_name IN (\"ssl_ca\",\"ssl_cert\",\"ssl_key\",\"require_secure_transport\");"'
#    若 ssl_cert 指向 /var/lib/mysql/... 或为空 → 我们的配置没被加载

# 2. 确认配置文件真的挂进了容器 conf.d
docker compose exec mysql sh -c 'cat /etc/mysql/conf.d/tls.cnf'

# 3. 确认 certs/ 文件在服务器上且 my.cnf 路径正确, 然后强制重建容器
docker compose up -d --force-recreate mysql

# 4. 重新探测, cert[0] 应变为 issuer=CN=mysqltls-demo CA 且带 IP SAN
```

**`x509: certificate signed by unknown authority`**

原因: 服务端用的不是本 CA 签的证书(例如上面那种 MySQL 自动生成证书)。
确认服务端 `ssl_cert` 用的是 `gen-ca.sh` 的产物并已生效(见上); 或换用
`MODE=pin` + `MYSQL_SPKI_FP`(只比对公钥指纹, 不看 CA 链)。

**`x509: cannot validate certificate for 103.42.30.173 because it doesn't contain any IP SANs`**

若 probe 显示证书 issuer 是 `CN=mysqltls-demo CA` 但仍报这条, 再分两种:

- **服务端出示的证书里一个 IP SAN 都没有** → 往往是把 `ca.pem` 误配成了
  `ssl_cert`, 或服务器上名为 `server.pem` 的文件内容其实是 CA 证书。把服务端
  `ssl_cert` 指回 `server.pem`:
  ```ini
  ssl_ca   = /certs/ca.pem
  ssl_cert = /certs/server.pem   # 必须是带 IP SAN 的服务端证书, 不是 ca.pem
  ssl_key  = /certs/server.key
  ```

- **服务端证书有 IP SAN 但没包含你的连接地址** → 用现有 CA 为访问地址重签
  (CA 不变, 客户端内嵌 ca.pem 无需重编):
  ```bash
  ./scripts/reissue-server-cert.sh 103.42.30.173
  # 把新 certs/server.pem(+server-bundle.pem) 覆盖到 MySQL 服务器, 重启 mysqld:
  #   物理机:  systemctl restart mysqld
  #   docker:  cp certs/server.pem 到服务器挂载目录后 docker compose restart mysql
  ```
