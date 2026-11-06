package initialize

import (
	"fmt"
	"os"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/gin-gonic/gin"
)

// ValidateStartupSecurity 在启动期阻断「示例配置直接上生产」。
//
// 只在 release 模式检查：debug 下保留示例值是为了本地零配置可跑；
// 而 release 模式一旦沿用公开的 jwt.signing-key，知道该值即可伪造任意用户的 JWT。
func ValidateStartupSecurity() {
	if gin.Mode() != gin.ReleaseMode {
		return
	}
	issues := global.GVA_CONFIG.ProductionSecretIssues()
	if len(issues) == 0 {
		return
	}
	const reason = "release 模式检测到占位或过短的 jwt.signing-key，拒绝启动；请先替换 server/config.yaml 中的签名密钥"
	// 同时写 stderr：容器里 zap 可能只落文件，启动失败原因必须能从 docker logs 直接看到。
	for _, issue := range issues {
		global.GVA_LOG.Error("启动配置不安全: " + issue)
		fmt.Fprintln(os.Stderr, "启动配置不安全: "+issue)
	}
	global.GVA_LOG.Error(reason)
	fmt.Fprintln(os.Stderr, reason)
	_ = global.GVA_LOG.Sync()
	os.Exit(1)
}
