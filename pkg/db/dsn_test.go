package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestRedactDSNMasksPassword(t *testing.T) {
	got, err := RedactDSN("root:secret@tcp(db.internal:8306)/shop?charset=utf8mb4&parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("密码未脱敏: %s", got)
	}
	if !strings.Contains(got, "root:***@tcp(db.internal:8306)/shop") {
		t.Errorf("脱敏后连接串不符合预期: %s", got)
	}
}

func TestRedactDSNInvalid(t *testing.T) {
	if _, err := RedactDSN("not-a-dsn"); err == nil {
		t.Fatal("非法 DSN 应当报错")
	}
}

func TestIsUnknownDatabase(t *testing.T) {
	if !IsUnknownDatabase(&mysql.MySQLError{Number: 1049, Message: "Unknown database 'x'"}) {
		t.Error("1049 应判定为库不存在")
	}
	if IsUnknownDatabase(&mysql.MySQLError{Number: 1045, Message: "Access denied"}) {
		t.Error("1045 不应判定为库不存在")
	}
	if IsUnknownDatabase(errors.New("network unreachable")) {
		t.Error("普通错误不应判定为库不存在")
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := QuoteIdent("plain"); got != "`plain`" {
		t.Errorf("QuoteIdent = %q", got)
	}
	// 内部反引号要转义成两个, 防止跳出标识符。
	if got := QuoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("反引号未转义: %q", got)
	}
}
