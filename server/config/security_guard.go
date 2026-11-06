package config

import (
	"fmt"
	"strings"
)

// minJWTSigningKeyLength 是 release 模式下接受的 JWT 签名密钥最小长度。
const minJWTSigningKeyLength = 16

// weakJWTSigningKeys 是公开仓库/示例配置里出现过的占位值（按小写比较）。
// 这些值任何人可见，知道即可伪造任意用户的 JWT，因此 release 模式必须拒绝。
var weakJWTSigningKeys = map[string]struct{}{
	"":                 {},
	"qmplus":           {},
	"gva":              {},
	"gin-vue-admin":    {},
	"secret":           {},
	"changeme":         {},
	"change-me":        {},
	"your-secret-key":  {},
	"your-signing-key": {},
	"123456":           {},
}

// ProductionSecretIssues 返回 release 模式下必须阻止启动的配置问题。
//
// 调用方只在 gin.ReleaseMode 下调用：debug 模式保留示例值是为了本地零配置可跑，
// 生产环境若沿用了公开的签名密钥，等于把登录态签发权交了出去。
func (c Server) ProductionSecretIssues() []string {
	key := strings.TrimSpace(c.JWT.SigningKey)
	if _, weak := weakJWTSigningKeys[strings.ToLower(key)]; weak {
		return []string{fmt.Sprintf("jwt.signing-key 使用了公开的占位值 %q", key)}
	}
	if len(key) < minJWTSigningKeyLength {
		return []string{fmt.Sprintf("jwt.signing-key 长度不足 %d 位（当前 %d 位）", minJWTSigningKeyLength, len(key))}
	}
	return nil
}
