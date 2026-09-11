package db

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"time"

	_ "github.com/go-sql-driver/mysql" // 触发 mysql driver 注册
)

// Options 是「账号密码直连」的封装入口: 只关心账号/密码/地址/库名,
// 证书校验(默认用内嵌的 certs/ca.pem)与连接池都在内部处理。
//
// 最简用法:
//
//	db, err := db.Connect("root", "pwd", "103.42.30.173:17941", "db_clife_employee")
//	if err != nil { return err }
//	defer db.Close()
//
// 需要更多控制时:
//
//	db, err := db.Options{
//	    User: "root", Password: "pwd",
//	    Addr: "103.42.30.173:17941", DB: "shop",
//	    SPKIFP: fp,          // 可选: 只钉服务端公钥指纹
//	    Insecure: true,      // 可选: 跳过证书校验(仅调试/隧道)
//	}.Open()
type Options struct {
	User     string
	Password string
	Addr     string // "host" 或 "host:port"; 省略端口默认 3306
	DB       string
	Net      string // 默认 "tcp"; 走 SSH 隧道传 "ssh"(需先 RegisterSSHDialer)

	CAPath   string // 可选: 覆盖内嵌的 certs/ca.pem
	SPKIFP   string // 可选: 设置后走公钥固定(pin), 忽略 CA 与主机名
	Insecure bool   // 可选: 跳过一切证书校验(仅调试/隧道)

	Params map[string]string // 额外 DSN 参数

	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Connect 是最简入口: 用内嵌 CA 以 verify-full 方式连接并 Ping 一次。
func Connect(user, password, addr, dbName string) (*sql.DB, error) {
	return Options{User: user, Password: password, Addr: addr, DB: dbName}.Open()
}

// DSN 构造连接串; 会按需把 TLS 预设注册进 driver(复用 RegisterTLSPresets)。
func (o Options) DSN() (string, error) {
	if o.User == "" {
		return "", fmt.Errorf("db: User 不能为空")
	}
	if err := RegisterTLSPresets(o.CAPath, o.SPKIFP); err != nil {
		return "", err
	}
	netProto := o.Net
	if netProto == "" {
		netProto = "tcp"
	}

	vals := url.Values{}
	vals.Set("tls", o.tlsProfile())
	vals.Set("parseTime", "true")
	vals.Set("charset", "utf8mb4")
	vals.Set("timeout", "10s")
	vals.Set("readTimeout", "30s")
	vals.Set("writeTimeout", "10s")
	for k, v := range o.Params {
		vals.Set(k, v)
	}

	return fmt.Sprintf("%s:%s@%s(%s)/%s?%s",
		o.User, o.Password, netProto, normalizeAddr(o.Addr), o.DB, vals.Encode()), nil
}

// Open 打开连接池并 Ping 一次; 失败会关掉池再返回错误。
func (o Options) Open() (*sql.DB, error) {
	dsn, err := o.DSN()
	if err != nil {
		return nil, err
	}
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: 打开连接失败: %w", err)
	}

	applyPool(d, o.MaxOpenConns, o.MaxIdleConns, o.ConnMaxLifetime, o.ConnMaxIdleTime)

	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("db: ping 失败: %w", err)
	}
	return d, nil
}

// tlsProfile 选择 RegisterTLSPresets 已注册好的具名 TLS 配置。
func (o Options) tlsProfile() string {
	switch {
	case o.SPKIFP != "":
		return "pin"
	case o.Insecure:
		return "skip-pinning"
	default:
		return "verify-full"
	}
}

// normalizeAddr 补默认端口 3306。
func normalizeAddr(addr string) string {
	if addr == "" {
		return "127.0.0.1:3306"
	}
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, "3306")
}

// applyPool 套用连接池上限; 上限不宜过大, 避免瞬间把 MySQL 打满,
// 限制单连接寿命可缩短凭据泄漏窗口。传 0 表示用默认值。
func applyPool(d *sql.DB, maxOpen, maxIdle int, life, idle time.Duration) {
	d.SetMaxOpenConns(orInt(maxOpen, 20))
	d.SetMaxIdleConns(orInt(maxIdle, 5))
	d.SetConnMaxLifetime(orDur(life, 30*time.Minute))
	d.SetConnMaxIdleTime(orDur(idle, 5*time.Minute))
}

func orInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func orDur(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}
