package jxc

import "github.com/flipped-aurora/gin-vue-admin/server/service"

type ApiGroup struct {
	BasicApi
	GoodsApi
	PurchaseApi
	SaleApi
	StockApi
	StockCheckApi
	CashierApi
	DashboardApi
	PosScanApi
	AiApi
}

var (
	basicService    = service.ServiceGroupApp.JxcServiceGroup.BasicService
	goodsService    = service.ServiceGroupApp.JxcServiceGroup.GoodsService
	purchaseService = service.ServiceGroupApp.JxcServiceGroup.PurchaseService
	saleService     = service.ServiceGroupApp.JxcServiceGroup.SaleService
	stockService    = service.ServiceGroupApp.JxcServiceGroup.StockService
	stockCheckService = service.ServiceGroupApp.JxcServiceGroup.StockCheckService
	cashierService  = service.ServiceGroupApp.JxcServiceGroup.CashierService
	dashboardService = service.ServiceGroupApp.JxcServiceGroup.DashboardService
	posScanService  = service.ServiceGroupApp.JxcServiceGroup.PosScanService
	aiService       = service.ServiceGroupApp.JxcServiceGroup.AiService
)
