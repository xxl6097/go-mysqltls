package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// OpenDSN 基于一条完整 DSN 打开连接池: 设好池上限并 Ping 一次。
//
// 若 DSN 指向的库不存在(MySQL 1049), 会自动 CREATE DATABASE 后用原 DSN 重连,
// 返回值 createdDB 表示这次是否新建了库 —— 首次部署时 DSN 常指向还没建的库。
//
//	conn, created, err := db.OpenDSN(ctx, "root:pwd@tcp(host:8306)/shop?charset=utf8mb4")
func OpenDSN(ctx context.Context, dsn string) (conn *sql.DB, createdDB bool, err error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, false, fmt.Errorf("db: DSN 解析失败: %w", err)
	}

	conn, err = newPool(dsn)
	if err == nil {
		err = conn.PingContext(ctx)
	}
	if err == nil {
		return conn, false, nil
	}
	// 只有「库不存在」才值得建库重连; 其它错误(密码错/网络不通/权限不足)直接上报。
	if !IsUnknownDatabase(err) || cfg.DBName == "" {
		conn.Close()
		return nil, false, fmt.Errorf("db: 连接失败: %w", err)
	}
	conn.Close()

	createdDB, err = EnsureDatabase(ctx, cfg)
	if err != nil {
		return nil, false, err
	}

	conn, err = newPool(dsn)
	if err != nil {
		return nil, createdDB, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, createdDB, fmt.Errorf("db: 建库后重连仍失败: %w", err)
	}
	return conn, createdDB, nil
}

// EnsureDatabase 保证 cfg.DBName 这个库存在, 不存在则创建。
// 返回 created 表示这次真的建了(用 RowsAffected 判断)。
//
// 注意: 目标库不存在时, 带库名的 DSN 连握手都过不去, 所以这里用一条
// 「去掉库名」的连接连上服务器再 CREATE DATABASE。
func EnsureDatabase(ctx context.Context, cfg *mysql.Config) (created bool, err error) {
	if cfg.DBName == "" {
		return false, errors.New("db: DBName 为空, 无需建库")
	}
	boot := *cfg
	boot.DBName = ""

	conn, err := newPool(boot.FormatDSN())
	if err != nil {
		return false, err
	}
	defer conn.Close()

	if err := conn.PingContext(ctx); err != nil {
		return false, fmt.Errorf("db: 连接服务器失败: %w", err)
	}

	q := "CREATE DATABASE IF NOT EXISTS " + QuoteIdent(cfg.DBName) + " CHARACTER SET utf8mb4"
	res, err := conn.ExecContext(ctx, q)
	if err != nil {
		return false, fmt.Errorf("db: 建库失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return true, nil // 建库成功但拿不到影响行数, 按已创建处理
	}
	return n > 0, nil
}

// IsUnknownDatabase 判断是否为 MySQL 的 1049 (Unknown database)。
func IsUnknownDatabase(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1049
}

// RedactDSN 把 DSN 里的密码替换成 *** 后返回, 便于打印/记日志。
func RedactDSN(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("db: DSN 解析失败: %w", err)
	}
	cfg.Passwd = "***"
	return cfg.FormatDSN(), nil
}

// QuoteIdent 用反引号包裹标识符并转义内部反引号, 避免拼接 SQL 注入。
func QuoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// newPool 打开连接池并套用默认上限。
func newPool(dsn string) (*sql.DB, error) {
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: 打开连接失败: %w", err)
	}
	applyPool(d, 0, 0, 0, 0)
	return d, nil
}
