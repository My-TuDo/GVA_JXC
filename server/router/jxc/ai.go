package jxc

import "github.com/gin-gonic/gin"

type AiRouter struct{}

// InitAiRouter AI 智能助手路由（挂 PrivateGroup：JWT + MustChangePwdGuard + Casbin + DataScope 全链）
func (r *AiRouter) InitAiRouter(Router *gin.RouterGroup) {
	// 只读组（不记操作日志）
	readOnlyGroup := Router.Group("jxc/ai")
	readOnlyGroup.POST("analyze", aiApi.Analyze)
	readOnlyGroup.POST("recommend", aiApi.Recommend)
}
