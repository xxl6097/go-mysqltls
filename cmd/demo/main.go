// Command demo 基于一条现成 DSN 连接 MySQL 并打印一轮连通性/信息自检。
// 逻辑都在 pkg/db 里(OpenDSN / Inspect), 这里只负责参数与环境变量。
//
//	go run ./cmd/demo
//	go run ./cmd/demo -dsn 'user:pass@tcp(host:8306)/db?charset=utf8mb4&parseTime=true&loc=Local'
//	MYSQL_DSN='...' go run ./cmd/demo
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/xxl6097/go-mysqltls/pkg/db"
)

// defaultDSN 只是本地测试默认值; 正式使用请用 -dsn 或 MYSQL_DSN 传入, 不要把密码写进源码/仓库。
const defaultDSN = "root:password@tcp(11.23.30.12:3306)/db_employee?charset=utf8mb4&parseTime=true&loc=Local"

func main() {
	dsn := flag.String("dsn", envOr("MYSQL_DSN", defaultDSN), "MySQL DSN")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// OpenDSN: 连接 + Ping, 目标库不存在时自动建库后重连。
	conn, createdDB, err := db.OpenDSN(ctx, *dsn)
	if err != nil {
		log.Fatalf("[connect] %v", err)
	}
	defer conn.Close()

	if redacted, err := db.RedactDSN(*dsn); err == nil {
		fmt.Printf("[dsn] %s\n", redacted)
	}
	if createdDB {
		fmt.Println("[db]   目标库不存在, 已自动创建")
	}
	fmt.Println("[ping] OK")

	info := db.Inspect(ctx, conn)
	if info.Version != "" {
		fmt.Printf("[ver]  %s\n", info.Version)
	}
	fmt.Printf("[db]   当前库 = %q\n", info.Database)
	fmt.Printf("[ssl]  cipher = %q\n", info.SSLCipher)
	if !info.Encrypted() {
		fmt.Println("[ssl]  WARNING: 当前连接未加密(明文), 敏感数据请改用 TLS")
	}

	if info.Database == "" {
		fmt.Printf("[dbs] 共 %d 个库\n", len(info.Databases))
		for _, d := range info.Databases {
			fmt.Printf("  - %s\n", d)
		}
	} else {
		fmt.Printf("[tables] 共 %d 张表\n", len(info.Tables))
		for i, t := range info.Tables {
			if i == 20 {
				fmt.Printf("  ...(其余 %d 张省略)\n", len(info.Tables)-i)
				break
			}
			fmt.Printf("  - %s\n", t)
		}
		if info.HasRowCount {
			fmt.Printf("[count] %s = %d 行\n", info.FirstTable, info.FirstTableRows)
		}
	}

	var one int
	if err := conn.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		log.Fatalf("[sql]  SELECT 1 失败: %v", err)
	}
	fmt.Printf("[sql]  SELECT 1 = %d\n", one)
	fmt.Println("OK - demo 跑通")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
