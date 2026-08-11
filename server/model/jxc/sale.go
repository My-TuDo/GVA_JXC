package jxc

import (
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
)

// SaleQuery 销售单分页查询（含日期范围，供看板历史统计下钻）
type SaleQuery struct {
	request.PageInfo
	From string `json:"from" form:"from"` // 起始日期 yyyy-mm-dd（含当天）
	To   string `json:"to" form:"to"`     // 结束日期 yyyy-mm-dd（含当天）
}

// 销售单状态
const (
	SaleStatusPending  = 1 // 待出库
	SaleStatusShipped  = 2 // 已出库
	SaleStatusCanceled = 3 // 已取消
)

// 销售单类型
const (
	SaleTypeNormal   = 1 // 正常销售
	SaleTypeReturn   = 2 // 退货退款（金额为负, 表示收入减少）
	SaleTypeExchange = 3 // 换货（一张单含换出 qty<0 与换入 qty>0 明细, 联合确认）
)

// POSItem 收银明细
type POSItem struct {
	SkuID uint `json:"skuId" binding:"required" comment:"SKU ID"`
	Qty   int  `json:"qty" binding:"required" comment:"数量"`
}

// POSCheckoutReq 收银结算请求（收款即出库）
type POSCheckoutReq struct {
	WarehouseID uint      `json:"warehouseId" binding:"required" comment:"收银仓库ID"`
	CustomerID  *uint     `json:"customerId" comment:"客户ID（可选，散客为空）"`
	PayMethod   string    `json:"payMethod" binding:"required" comment:"支付方式 cash/wechat/alipay"`
	PaidAmount  float64   `json:"paidAmount" comment:"实收金额（现金找零用）"`
	Items       []POSItem `json:"items" binding:"required" comment:"明细列表"`
	Remark      string    `json:"remark" comment:"备注"`
}

// POSRefundReq 收银退款请求（生成退货单并直接确认入库）
type POSRefundReq struct {
	OriginalOrderID uint      `json:"originalOrderId" binding:"required" comment:"原销售单ID"`
	Items           []POSItem `json:"items" binding:"required" comment:"退款明细（原单出过的商品）"`
	Remark          string    `json:"remark" comment:"备注"`
}

// POSExchangeReq 收银换货请求（退回商品入库 + 换出商品出库，差额多退少补）
type POSExchangeReq struct {
	OriginalOrderID uint      `json:"originalOrderId" binding:"required" comment:"原销售单ID"`
	WarehouseID     uint      `json:"warehouseId" comment:"换出仓库（默认原单仓库）"`
	CustomerID      *uint     `json:"customerId" comment:"客户ID（可选）"`
	PayMethod       string    `json:"payMethod" binding:"required" comment:"支付方式 cash/wechat/alipay"`
	PaidAmount      float64   `json:"paidAmount" comment:"实收金额（差额为正时现金校验）"`
	ReturnItems     []POSItem `json:"returnItems" binding:"required" comment:"退回商品（原单商品）"`
	OutItems        []POSItem `json:"outItems" binding:"required" comment:"换出商品（新商品）"`
	Remark          string    `json:"remark" comment:"备注"`
}

// POSExchangeResult 收银换货结果
type POSExchangeResult struct {
	ReturnOrder *SaleOrder `json:"returnOrder" comment:"退货单（已入库，金额为负）"`
	OutOrder    *SaleOrder `json:"outOrder" comment:"销售单（已出库）"`
	DiffAmount  float64    `json:"diffAmount" comment:"差额（正=应收，负=应退）"`
}

// SaleRemaining 原单剩余可退换件数（供前端数量上限钳制）
type SaleRemaining struct {
	OrderNo   string `json:"orderNo" gorm:"-" comment:"原单单号"`
	OutQty    int    `json:"outQty" gorm:"-" comment:"原单出库件数"`
	UsedQty   int    `json:"usedQty" gorm:"-" comment:"已被确认单据占用件数"`
	Remaining int    `json:"remaining" gorm:"-" comment:"剩余可退换件数"`
}

// SaleOrder 销售单
type SaleOrder struct {
	global.GVA_MODEL
	OrderNo         string  `json:"orderNo" gorm:"column:order_no;size:50;not null;uniqueIndex:uk_sale_order_no;comment:销售单号"`
	CustomerID      *uint   `json:"customerId" gorm:"column:customer_id;index:idx_sale_customer_id;comment:客户ID（零售可空）"`
	WarehouseID     uint    `json:"warehouseId" gorm:"column:warehouse_id;not null;comment:出库仓库ID"`
	OrderType       int8    `json:"orderType" gorm:"column:order_type;not null;default:1;comment:单据类型 1正常销售 2退货退款 3换货"`
	OriginalOrderID *uint   `json:"originalOrderId" gorm:"column:original_order_id;index:idx_sale_original_order_id;comment:关联原单ID（退货/换货）"`
	Status          int8    `json:"status" gorm:"column:status;not null;default:1;comment:状态 1待出库 2已出库 3已取消"`
	TotalAmount     float64 `json:"totalAmount" gorm:"column:total_amount;type:decimal(12,2);not null;default:0;comment:销售总金额"`
	Remark          string  `json:"remark" gorm:"column:remark;size:500;comment:备注"`
	Creator         string  `json:"creator" gorm:"column:creator;size:50;comment:创建人"`

	// 关联
	Customer  *Customer   `json:"customer" gorm:"foreignKey:CustomerID;references:ID"`
	Warehouse *Warehouse  `json:"warehouse" gorm:"foreignKey:WarehouseID;references:ID"`
	Original  *SaleOrder  `json:"original" gorm:"foreignKey:OriginalOrderID;references:ID"`
	Items     []SaleItem  `json:"items" gorm:"foreignKey:SaleID;references:ID"`
}

func (SaleOrder) TableName() string { return "sale_order" }

// SaleItem 销售明细（生命周期跟随销售单，不软删除）
type SaleItem struct {
	ID        uint    `json:"ID" gorm:"primarykey;comment:主键ID"`
	SaleID    uint    `json:"saleId" gorm:"column:sale_id;not null;index:idx_sale_item_order;comment:销售单ID"`
	SkuID     uint    `json:"skuId" gorm:"column:sku_id;not null;comment:SKU ID"`
	SkuCode   string  `json:"skuCode" gorm:"column:sku_code;size:50;comment:SKU编码快照"`
	GoodsName string  `json:"goodsName" gorm:"column:goods_name;size:200;comment:商品名称快照"`
	Color     string  `json:"color" gorm:"column:color;size:50;comment:颜色快照"`
	Size      string  `json:"size" gorm:"column:size;size:50;comment:尺码快照"`
	Qty       int     `json:"qty" gorm:"column:qty;not null;default:0;comment:数量（恒为正）"`
	Price     float64 `json:"price" gorm:"column:price;type:decimal(12,2);not null;default:0;comment:销售单价"`
	Amount    float64 `json:"amount" gorm:"column:amount;type:decimal(12,2);not null;default:0;comment:金额（退货/换入为负）"`
	Direction int8    `json:"direction" gorm:"column:direction;not null;default:0;comment:方向 0默认 1换出(正) 2换入(负)"`

	Sku *GoodsSku `json:"sku" gorm:"foreignKey:SkuID;references:ID"`
}

func (SaleItem) TableName() string { return "sale_item" }
