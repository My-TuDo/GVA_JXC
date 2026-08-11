package jxc

type ServiceGroup struct {
	BasicService
	GoodsService
	PurchaseService
	SaleService
	StockService
	StockCheckService
	CashierService
	DashboardService
	PosScanService
	AiService
}

var JxcServiceGroup = new(ServiceGroup)
