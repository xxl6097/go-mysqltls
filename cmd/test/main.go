// Package main 演示用 go-sql-driver/mysql 安全连接 MySQL 的几种方式:
//
//	MODE=tls  : TLS + CA 校验   (生产首选)
//	MODE=pin  : TLS + 公钥固定  (防 CA 被钓/吊销)
//	MODE=ssh  : SSH 隧道       (不改 MySQL 配置也能加密)
//
// 详见 README.md。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/xxl6097/go-mysqltls/pkg/db"
)

func main() {
	mode := flag.String("mode", envOr("MODE", "tls"), "tls | pin | ssh")
	flag.Parse()

	cfg, err := db.LoadConfig()
	if err != nil {
		log.Fatalf("[config] %v", err)
	}

	fmt.Println(cfg)
	if err := db.Run(cfg, *mode); err != nil {
		log.Fatalf("[connect] %v", err)
	}
	fmt.Println("OK - connection is up & query works")
}

func envOr(k, def string) string {
	if v := lookup(k); v != "" {
		return v
	}
	return def
}

func lookup(k string) string { return os.Getenv(k) }
