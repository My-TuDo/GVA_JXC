package jxc

import (
	api "github.com/flipped-aurora/gin-vue-admin/server/api/v1"
)

type RouterGroup struct {
	BasicRouter
	GoodsRouter
	PurchaseRouter
	SaleRouter
	StockRouter
	StockCheckRouter
	CashierRouter
	DashboardRouter
	PosRouter
	AiRouter
}

var (
	basicApi    = api.ApiGroupApp.JxcApiGroup.BasicApi
	goodsApi    = api.ApiGroupApp.JxcApiGroup.GoodsApi
	purchaseApi = api.ApiGroupApp.JxcApiGroup.PurchaseApi
	saleApi     = api.ApiGroupApp.JxcApiGroup.SaleApi
	stockApi    = api.ApiGroupApp.JxcApiGroup.StockApi
	stockCheckApi = api.ApiGroupApp.JxcApiGroup.StockCheckApi
	cashierApi  = api.ApiGroupApp.JxcApiGroup.CashierApi
	dashboardApi = api.ApiGroupApp.JxcApiGroup.DashboardApi
	posApi      = api.ApiGroupApp.JxcApiGroup.PosScanApi
	aiApi       = api.ApiGroupApp.JxcApiGroup.AiApi
)
