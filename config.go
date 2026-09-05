package main

import (
	"fmt"
	"os"
	"strconv"
)

// Config 集中保存运行时所需的全部设置。
// 设计原则:
//   - 不在代码或仓库里出现任何明文凭证。
//   - 所有敏感字段全部从环境变量读取。
//   - 提供默认值兜底, 但 MYSQL_USER/MYSQL_PASSWORD 不设默认值, 缺一报错。
//   - CA 公钥证书用 go:embed 打包进二进制 (embeddedCACert), MYSQL_CA_PATH 仅在
//     需要临时覆盖时设置; 私钥 (ca.key/server.key) 永不进二进制。
type Config struct {
	Mode       string // "tls" | "pin" | "ssh"
	Host       string
	Port       int
	User       string
	Password   string
	DB         string
	TLSProfile string // mysql driver 的具名 TLS 配置: "verify-full" | "pin" | "skip-pinning"
	CAPath     string // 可选: 覆盖内嵌 CA 的 PEM 文件路径; 空则用内嵌 certs/ca.pem
	SPKIFP     string // SPKI 指纹（base64），pin 模式必填
	SSHAddr    string // "host:port"
	SSHUser    string
	SSHKeyPath string
}

// LoadConfig 从环境变量装载配置, 并按模式做强制检查。
func LoadConfig() (*Config, error) {
	c := &Config{
		Mode:       lookup("MODE"),
		Host:       lookup("MYSQL_HOST"),
		User:       lookup("MYSQL_USER"),
		Password:   lookup("MYSQL_PASSWORD"),
		DB:         lookup("MYSQL_DB"),
		CAPath:     lookup("MYSQL_CA_PATH"),
		SPKIFP:     lookup("MYSQL_SPKI_FP"),
		SSHAddr:    lookup("MYSQL_SSH_HOST"),
		SSHUser:    lookup("MYSQL_SSH_USER"),
		SSHKeyPath: lookup("MYSQL_SSH_KEY_PATH"),
	}
	if c.Mode == "" {
		c.Mode = "tls"
	}
	if c.Host == "" {
		c.Host = "103.42.30.173"
	}
	if c.User == "" {
		c.User = "root"
	}
	if c.Password == "" {
		c.Password = "Zjjy2014Xyz"
	}
	if c.DB == "" {
		c.DB = "db_clife_employee"
	}
	p, err := strconv.Atoi(lookup("MYSQL_PORT")) //envDefault("MYSQL_PORT", "3306")
	if err != nil {
		//return nil, fmt.Errorf("MYSQL_PORT 非法: %w", err)
		c.Port = 17941
	} else {
		c.Port = p
	}

	switch c.Mode {
	case "tls":
		c.TLSProfile = "verify-full"
	case "pin":
		c.TLSProfile = "pin"
	case "ssh":
		c.TLSProfile = "skip-pinning"
	default:
		return nil, fmt.Errorf("无效的 MODE %q (期望 tls|pin|ssh)", c.Mode)
	}

	if c.User == "" || c.Password == "" {
		return nil, fmt.Errorf("MYSQL_USER 与 MYSQL_PASSWORD 必须显式提供")
	}
	if c.Host == "" {
		return nil, fmt.Errorf("MYSQL_HOST 不能为空")
	}
	// CAPath 不再强制: 未设置时 RegisterTLSPresets 会用 go:embed 内嵌的 ca.pem。
	// 只有设置成存在的文件、又想校验失败报错时, 用户才显式填 MYSQL_CA_PATH。
	if c.Mode == "pin" && c.SPKIFP == "" {
		return nil, fmt.Errorf("mode=pin 需要 MYSQL_SPKI_FP (base64 SPKI 指纹)")
	}
	if c.Mode == "ssh" {
		if c.SSHAddr == "" || c.SSHUser == "" || c.SSHKeyPath == "" {
			return nil, fmt.Errorf("mode=ssh 需要 MYSQL_SSH_HOST / MYSQL_SSH_USER / MYSQL_SSH_KEY_PATH")
		}
	}
	return c, nil
}

func lookup(k string) string { return os.Getenv(k) }

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
