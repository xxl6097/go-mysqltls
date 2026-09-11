package db

import (
	"context"
	"database/sql"
)

// Info 是一次连接自检的结果。各项均为「尽力而为」: 某一步查不到就留零值, 不中断。
type Info struct {
	Version        string   // SELECT VERSION()
	Database       string   // SELECT DATABASE(); 未选库时为空
	SSLCipher      string   // Ssl_cipher; 空表示当前连接未加密
	Databases      []string // 仅当未选库时填充: SHOW DATABASES
	Tables         []string // SHOW TABLES
	FirstTable     string   // Tables[0]
	FirstTableRows int64    // FirstTable 的行数
	HasRowCount    bool     // FirstTableRows 是否有效
}

// Encrypted 报告当前连接是否真的走了 TLS。
func (i Info) Encrypted() bool { return i.SSLCipher != "" }

// Inspect 对一条已建立的连接做一轮信息探测: 版本 / 当前库 / 是否加密 /
// (未选库时)库清单 / 表清单 / 首表行数。
func Inspect(ctx context.Context, conn *sql.DB) *Info {
	info := &Info{}
	var v sql.NullString

	if err := conn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err == nil {
		info.Version = v.String
	}
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&v); err == nil {
		info.Database = v.String
	}

	// SHOW STATUS 返回 (Variable_name, Value) 两列。
	var name, cipher sql.NullString
	if err := conn.QueryRowContext(ctx, "SHOW STATUS LIKE 'Ssl_cipher'").Scan(&name, &cipher); err == nil {
		info.SSLCipher = cipher.String
	}

	// 没选库时列库, 选了库时列表。
	if info.Database == "" {
		if dbs, err := Databases(ctx, conn); err == nil {
			info.Databases = dbs
		}
		return info
	}
	tables, err := Tables(ctx, conn)
	if err != nil {
		return info
	}
	info.Tables = tables
	if len(tables) > 0 {
		info.FirstTable = tables[0]
		if n, err := CountRows(ctx, conn, tables[0]); err == nil {
			info.FirstTableRows, info.HasRowCount = n, true
		}
	}
	return info
}

// Databases 返回服务器上的库清单 (SHOW DATABASES)。
func Databases(ctx context.Context, conn *sql.DB) ([]string, error) {
	return scanStrings(conn.QueryContext(ctx, "SHOW DATABASES"))
}

// Tables 返回当前库的表清单 (SHOW TABLES)。
func Tables(ctx context.Context, conn *sql.DB) ([]string, error) {
	return scanStrings(conn.QueryContext(ctx, "SHOW TABLES"))
}

// CountRows 返回某张表的行数。表名用反引号包裹并转义, 不做裸拼接。
func CountRows(ctx context.Context, conn *sql.DB, table string) (int64, error) {
	var n int64
	err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+QuoteIdent(table)).Scan(&n)
	return n, err
}

// scanStrings 把单列的结果集收成字符串切片。
func scanStrings(rows *sql.Rows, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
