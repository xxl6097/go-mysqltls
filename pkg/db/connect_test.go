package db

import (
	"strings"
	"testing"
)

func TestOptionsDSNProfile(t *testing.T) {
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"默认内嵌CA", Options{User: "u"}, "tls=verify-full"},
		{"公钥固定", Options{User: "u", SPKIFP: "AAAA="}, "tls=pin"},
		{"跳过校验", Options{User: "u", Insecure: true}, "tls=skip-pinning"},
	}
	for _, c := range cases {
		dsn, err := c.o.DSN()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(dsn, c.want) {
			t.Errorf("%s: DSN 缺少 %q:\n%s", c.name, c.want, dsn)
		}
	}
}

func TestOptionsDSNAddrAndDefaults(t *testing.T) {
	dsn, err := Options{User: "root", Password: "p", Addr: "10.0.0.5", DB: "shop"}.DSN()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"root:p@tcp(10.0.0.5:3306)/shop", "charset=utf8mb4", "parseTime=true"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN 缺少 %q:\n%s", want, dsn)
		}
	}
}

func TestOptionsDSNKeepsExplicitPort(t *testing.T) {
	dsn, err := Options{User: "u", Addr: "db.internal:17941"}.DSN()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "@tcp(db.internal:17941)/") {
		t.Errorf("端口未保留:\n%s", dsn)
	}
}

func TestOptionsDSNSSHNet(t *testing.T) {
	dsn, err := Options{User: "u", Net: "ssh", Addr: "mysql.internal:3306", Insecure: true}.DSN()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "@ssh(mysql.internal:3306)/") {
		t.Errorf("ssh 协议未生效:\n%s", dsn)
	}
}

func TestOptionsUserRequired(t *testing.T) {
	if _, err := (Options{Addr: "127.0.0.1"}).DSN(); err == nil {
		t.Fatal("User 为空时应当报错")
	}
}

func TestOptionsCustomCAPathMissing(t *testing.T) {
	if _, err := (Options{User: "u", CAPath: "/nonexistent/ca.pem"}).DSN(); err == nil {
		t.Fatal("显式指定的 CA 路径不存在时应报错, 不应静默回退")
	}
}
