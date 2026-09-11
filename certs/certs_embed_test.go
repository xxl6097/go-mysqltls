// 用外部测试包(certs_test), 否则 certs → db → certs 会构成 import cycle。
package certs_test

import (
	"testing"

	"github.com/xxl6097/go-mysqltls/certs"
	"github.com/xxl6097/go-mysqltls/pkg/db"
)

// TestEmbeddedCALoadsAndRegisters 验证: 不设 CA 路径时, go:embed 的 ca.pem 能被
// 读出来并成功注册三套 TLS 预设 (不发起任何网络连接)。
func TestEmbeddedCALoadsAndRegisters(t *testing.T) {
	if len(certs.EmbeddedCACert) == 0 {
		t.Fatal("EmbeddedCACert 为空: 请确认 certs/ca.pem 存在 (先跑 ./scripts/gen-ca.sh)")
	}
	if err := db.RegisterTLSPresets("", "dGVzdA=="); err != nil {
		t.Fatalf("使用内嵌 CA 注册 TLS 预设失败: %v", err)
	}
}

// TestExternalCAPathMissingFails 验证: 用户显式填了 MYSQL_CA_PATH 但文件不存在时,
// 必须报错而不是静默回退到内嵌 CA —— 覆盖场景里填错路径要比"悄悄用旧 CA"安全。
func TestExternalCAPathMissingFails(t *testing.T) {
	if err := db.RegisterTLSPresets("/nonexistent/ca.pem", ""); err == nil {
		t.Fatal("期望显式指定的 CA 路径不存在时报错, 实际无错误")
	}
}
