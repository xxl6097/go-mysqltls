package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql" // 触发 mysql driver 注册
)

// Run 是入口的统一执行单元:
//   1. 注册 TLS 预设/ssh 拨号器
//   2. 打开连接, 设置连接池上限
//   3. Ping 触发实际拨号
//   4. 读 Ssl_cipher 确认是不是真的加密
//   5. 跑一条简单的 SELECT 验证通路
func Run(cfg *Config, mode string) error {
	if err := RegisterTLSPresets(cfg.CAPath, cfg.SPKIFP); err != nil {
		return err
	}
	if mode == "ssh" {
		if err := RegisterSSHDialer(cfg.SSHAddr, cfg.SSHUser, cfg.SSHKeyPath); err != nil {
			return err
		}
	}

	netProto := "tcp"
	if mode == "ssh" {
		netProto = "ssh"
	}
	dsn := fmt.Sprintf(
		"%s:%s@%s(%s:%d)/%s?tls=%s&parseTime=true&charset=utf8mb4"+
			"&timeout=10s&readTimeout=30s&writeTimeout=10s",
		cfg.User, cfg.Password,
		netProto, cfg.Host, cfg.Port,
		cfg.DB, cfg.TLSProfile,
	)
	fmt.Printf("[dial] %s\n", redactDSN(dsn))

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开连接失败: %w", err)
	}
	defer db.Close()

	// 连接池参数: 上限不宜过大, 避免瞬间把 MySQL 打满;
	// 限制单连接寿命, 缩短凭据泄漏窗口。
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping 失败: %w", err)
	}

	// 用 MySQL 自带的会话变量校验加密真的生效 (空字符串表示未加密)。
	var cipher sql.NullString
	if err := db.QueryRow("SHOW STATUS LIKE 'Ssl_cipher'").Scan(new(string), &cipher); err == nil {
		fmt.Printf("[ssl]  cipher = %q\n", cipher.String)
		if !cipher.Valid || cipher.String == "" {
			fmt.Println("[ssl]  WARNING: 连接未加密! 请检查 tls= 设置 / 服务端 ssl 配置")
		}
	}

	// 真正的查询示例, 用 sql 占位符避免任何拼接注入风险。
	var v int
	if err := db.QueryRow("SELECT 1").Scan(&v); err != nil {
		return fmt.Errorf("select 失败: %w", err)
	}
	fmt.Printf("[sql]  SELECT 1 = %d\n", v)
	return nil
}

// redactDSN 把 DSN 里的密码掩掉再打印, 避免在终端泄露。
func redactDSN(dsn string) string {
	// 简单匹配 "user:password@", 不追求完美.
	out := []rune{}
	inPwd := false
	for i, r := range dsn {
		switch {
		case i > 0 && r == ':' && !inPwd && containsUserSep(dsn, i):
			inPwd = true
			out = append(out, r)
		case inPwd && r == '@':
			out = append(out, '*', '*', '*', r)
			inPwd = false
		case inPwd:
			// 吞掉密码
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func containsUserSep(_ string, _ int) bool { return true } // 简化: 见到 : 就视作密码起点
