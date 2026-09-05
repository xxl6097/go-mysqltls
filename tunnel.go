package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// RegisterSSHDialer 在 go-sql-driver/mysql 里注册自定义协议 "ssh"。
// 用法 (DSN): user:pass@ssh(real-mysql-host:3306)/db
//
// 行为: 当 driver 解析到 "ssh(host:port)" 时, 会调用本函数返回的 dialer:
//  1. 用本机 SSH 私钥登录跳板机;
//  2. 通过加密隧道连接到真正的 MySQL 主机;
//  3. 在加密隧道内, MySQL 明文也能避免被网络抓包 (类似 ssh -L 端口转发)。
func RegisterSSHDialer(sshAddr, sshUser, sshKeyPath string) error {
	key, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return fmt.Errorf("读取 SSH 私钥失败 %q: %w", sshKeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("解析 SSH 私钥失败: %w", err)
	}

	khPath := envDefault("MYSQL_SSH_KNOWN_HOSTS", os.Getenv("HOME")+"/.ssh/known_hosts")
	kh, err := knownhosts.New(khPath)
	if err != nil {
		return fmt.Errorf("加载 known_hosts 失败 %q: %w", khPath, err)
	}

	sshCfg := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// 用 known_hosts 做主机密钥校验, 防止跳板机被冒名。
		HostKeyCallback: kh,
	}

	mysql.RegisterDialContext("ssh", func(ctx context.Context, addr string) (net.Conn, error) {
		client, err := ssh.Dial("tcp", sshAddr, sshCfg)
		if err != nil {
			return nil, fmt.Errorf("SSH 登录 %s 失败: %w", sshAddr, err)
		}
		// addr 就是 DSN 里的 mysql 服务器真实地址。
		return client.DialContext(ctx, "tcp", addr)
	})
	return nil
}
