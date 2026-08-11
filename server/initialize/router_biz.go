package initialize

import (
	"github.com/flipped-aurora/gin-vue-admin/server/router"
	"github.com/gin-gonic/gin"
)

// 占位方法，保证文件可以正确加载，避免go空变量检测报错，请勿删除。
func holder(routers ...*gin.RouterGroup) {
	_ = routers
	_ = router.RouterGroupApp
}

func initBizRouter(routers ...*gin.RouterGroup) {
	privateGroup := routers[0]
	publicGroup := routers[1]

	// jxc 基础资料路由
	router.RouterGroupApp.Jxc.InitBasicRouter(privateGroup)
	// jxc AI 智能助手路由（需登录+权限，挂 PrivateGroup）
	router.RouterGroupApp.Jxc.InitAiRouter(privateGroup)
	// jxc Goods 路由
	router.RouterGroupApp.Jxc.InitGoodsRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitPurchaseRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitSaleRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitStockRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitStockCheckRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitCashierRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitDashboardRouter(publicGroup)
	router.RouterGroupApp.Jxc.InitPosRouter(publicGroup)

	holder(publicGroup, privateGroup)

}
