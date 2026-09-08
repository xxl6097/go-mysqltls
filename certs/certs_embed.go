package certs

import (
	_ "embed"
)

//go:embed ca.pem
var EmbeddedCACert []byte

// 说明: 只 embed CA 公钥证书 (ca.pem), 绝不把 ca.key / server.key 等私钥编进二进制。
// 私钥一旦进二进制, 任何拿到程序的人都能提取出来, 等于私钥泄露。
// 需要生成/更新证书时, 跑 ./scripts/gen-ca.sh 再重新 build 即可。
