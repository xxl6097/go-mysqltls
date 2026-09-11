package db

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/go-sql-driver/mysql"
	"github.com/xxl6097/go-mysqltls/certs"
)

// RegisterTLSPresets 把三套 TLS 配置注册进 go-sql-driver/mysql:
//
//	"verify-full" —— 标准 CA 校验; 主机名校验由 driver 用 DSN 中的 host 自动注入。
//	"pin"        —— 跳过 CA 链校验, 自己比对服务端 SPKI 指纹, 防 CA 被钓/被吊销。
//	"skip-pinning" —— 不做应用层证书校验, 仅用于 SSH 隧道场景(SSh 已加密)。
//
// caPath 为空时使用 go:embed 打进二进制里的 certs/ca.pem (embeddedCACert),
// 这样编译出的单文件可执行程序不用再额外带证书文件; 需要临时换 CA 时
// 才显式设置 MYSQL_CA_PATH 指向外部 PEM。
func RegisterTLSPresets(caPath, spkiFP string) error {
	caPEM, err := loadCAPEM(caPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("CA 文件无效: %s", caPath)
	}

	// 1. 方案二: CA 校验 + 主机名校验 (推荐)
	mysql.RegisterTLSConfig("verify-full", &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	})

	// 2. 方案四: SPKI 钉住
	if spkiFP != "" {
		mysql.RegisterTLSConfig("pin", &tls.Config{
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				if len(rawCerts) == 0 {
					return fmt.Errorf("服务端未返回证书")
				}
				cert, err := x509.ParseCertificate(rawCerts[0])
				if err != nil {
					return fmt.Errorf("解析服务端证书失败: %w", err)
				}
				sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
				got := base64.StdEncoding.EncodeToString(sum[:])
				if got != spkiFP {
					return fmt.Errorf("公钥指纹不匹配: 期望 %s, 实际 %s", spkiFP, got)
				}
				return nil
			},
			MinVersion: tls.VersionTLS12,
		})
	}

	// 3. ssh 模式下的兜底 TLS (SSH 隧道本身已加密, 应用层可以不再校验)
	mysql.RegisterTLSConfig("skip-pinning", &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	return nil
}

// loadCAPEM 优先读外部文件; 未设置路径时回落到内嵌的 CA 公钥证书。
// 如果用户显式指定了路径, 文件必须存在, 否则直接报错, 不静默回退。
func loadCAPEM(caPath string) ([]byte, error) {
	if caPath == "" {
		return certs.EmbeddedCACert, nil
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("读取 CA 文件失败 %q: %w", caPath, err)
	}
	return caPEM, nil
}
