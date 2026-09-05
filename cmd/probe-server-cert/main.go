// Command probe-server-cert 打印 MySQL 服务端实际出示的 TLS 证书链。
//
// 用途: 排查 "x509: cannot validate certificate for <ip> because it
// doesn't contain any IP SANs" 这类问题 —— 直接看服务器到底在出示哪张证书。
//
// 实现: 复用 go-sql-driver/mysql 的标准 SSLRequest/TLS 握手, 在
// VerifyPeerCertificate 回调里打印证书(用假凭据, TLS 在鉴权之前完成)。
//
// 用法(在能连通 MySQL:3306 的机器上跑):
//
//	go run ./cmd/probe-server-cert 103.42.30.173:3306
//
// 判断:
//   - 若 cert[0] 的 IP SANs 为空且 subject 含 "mysqltls-demo CA",
//     说明服务端把 ca.pem 当成了 ssl_cert → 把服务端 ssl_cert 改回 server.pem。
//   - 若 cert[0] 的 IP SANs 里没有你的连接地址 → 用 reissue-server-cert.sh 重签。
//   - 若 issuer 不是 "mysqltls-demo CA" → 服务端没用本 CA 的证书。
package main

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./cmd/probe-server-cert host:port")
		os.Exit(1)
	}
	hostport := os.Args[1]

	err := mysql.RegisterTLSConfig("probe", &tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					fmt.Printf("cert[%d] parse err: %v\n", i, err)
					continue
				}
				fmt.Printf("--- cert[%d] ---\n", i)
				fmt.Printf("  subject: %s\n", c.Subject)
				fmt.Printf("  issuer : %s\n", c.Issuer)
				fmt.Printf("  DNS SANs: %v\n", c.DNSNames)
				fmt.Printf("  IP  SANs: %v\n", c.IPAddresses)
			}
			return nil
		},
	})
	if err != nil {
		fmt.Println("register TLS config err:", err)
		os.Exit(1)
	}

	cfg := mysql.NewConfig()
	cfg.User = "probe"
	cfg.Passwd = "probe" // 假凭据: TLS 握手发生在鉴权之前, 足够拿到证书
	cfg.Net = "tcp"
	cfg.Addr = hostport
	cfg.TLSConfig = "probe"
	cfg.AllowNativePasswords = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		fmt.Println("sql.Open err:", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		// 假凭据, 鉴权失败是预期内的; 证书已经在上面的回调里打印了。
		fmt.Println("ping err(预期,假凭据):", err)
	}
	fmt.Println("说明: cert[] 即服务器实际出示的证书。若上面没有 cert[] 却报错, 说明 TLS 握手都没成功。")
}
