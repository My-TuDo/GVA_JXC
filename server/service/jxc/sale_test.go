package jxc

import (
	"context"
	"strings"
	"errors"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/internal/testutil"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

var saleSvc = new(SaleService)

// saleTestDB 销售模块测试库（基础+商品+SKU+采购+库存+销售）
func saleTestDB(t *testing.T) {
	t.Helper()
	testutil.NewMemoryDB(t,
		&jxc.GoodsCategory{}, &jxc.Brand{}, &jxc.Supplier{}, &jxc.Customer{}, &jxc.Warehouse{},
		&jxc.Goods{}, &jxc.GoodsSku{},
		&jxc.PurchaseOrder{}, &jxc.PurchaseItem{}, &jxc.Stock{}, &jxc.StockLog{},
		&jxc.SaleOrder{}, &jxc.SaleItem{},
	)
}

// newSaleFixture 前置：客户/仓库/商品/SKU + 库存 10 件，返回 skuID
func newSaleFixture(t *testing.T) uint {
	t.Helper()
	db := global.GVA_DB
	db.Create(&jxc.Customer{Code: "KH001", Name: "测试客户", Status: 1})
	db.Create(&jxc.Warehouse{Code: "CK001", Name: "总仓", Status: 1})
	g := jxc.Goods{Code: "SP001", Name: "测试商品", Status: 1}
	db.Create(&g)
	sku := jxc.GoodsSku{GoodsID: g.ID, SkuCode: "SP001-M-红", Color: "红", Size: "M", CostPrice: 8, SalePrice: 15, Status: 1}
	db.Create(&sku)
	db.Create(&jxc.Stock{WarehouseID: 1, SkuID: sku.ID, Quantity: 10})
	return sku.ID
}

// getStockQty 读取某 SKU 库存（quantity/lock_quantity）
func getStockQty(t *testing.T, skuID uint) (qty, lock int) {
	t.Helper()
	var stock jxc.Stock
	if err := global.GVA_DB.Where("warehouse_id = 1 AND sku_id = ?", skuID).First(&stock).Error; err != nil {
		t.Fatalf("读取库存失败: %v", err)
	}
	return stock.Quantity, stock.LockQuantity
}

// TestCreateSaleOrder_Lock 测试建单锁定库存（单价强制取 SKU 销售价）
func TestCreateSaleOrder_Lock(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	order := &jxc.SaleOrder{
		WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 99}}, // 前端传 99，应被强制为 SKU 销售价 15
	}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err != nil {
		t.Fatalf("创建销售单失败: %v", err)
	}
	if order.Status != jxc.SaleStatusPending {
		t.Errorf("初始状态应为待出库, got %d", order.Status)
	}
	if order.TotalAmount != 45 {
		t.Errorf("总金额应为 45（3*15, 强制销售价）, got %v", order.TotalAmount)
	}
	qty, lock := getStockQty(t, skuID)
	if qty != 10 || lock != 3 {
		t.Errorf("锁定后应为 qty=10 lock=3, got qty=%d lock=%d", qty, lock)
	}
	var items []jxc.SaleItem
	global.GVA_DB.Where("sale_id = ?", order.ID).Find(&items)
	if len(items) != 1 || items[0].SkuCode != "SP001-M-红" {
		t.Errorf("明细快照未写入: %+v", items)
	}
	if items[0].Price != 15 {
		t.Errorf("明细单价应强制为 SKU 销售价 15, got %v", items[0].Price)
	}
}

// TestCreateSaleOrder_Insufficient 测试库存不足拒绝且不锁定
func TestCreateSaleOrder_Insufficient(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	order := &jxc.SaleOrder{
		WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 11, Price: 15}},
	}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err == nil {
		t.Fatal("库存不足应报错")
	}
	_, lock := getStockQty(t, skuID)
	if lock != 0 {
		t.Errorf("失败创建不应锁定, lock=%d", lock)
	}
	// 无库存记录
	saleTestDB2 := func() {}
	_ = saleTestDB2
	var cnt int64
	global.GVA_DB.Model(&jxc.SaleOrder{}).Count(&cnt)
	if cnt != 0 {
		t.Error("失败创建不应产生销售单")
	}
}

// TestCreateSaleOrder_NoStock 测试无库存记录拒绝
func TestCreateSaleOrder_NoStock(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	global.GVA_DB.Exec("DELETE FROM stock")
	order := &jxc.SaleOrder{
		WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}},
	}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err == nil {
		t.Error("无库存记录应报错")
	}
}

// TestConfirmOut 测试出库（扣减+流水+状态）
func TestConfirmOut(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := saleSvc.ConfirmOut(ctx, order.ID, "tester"); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	qty, lock := getStockQty(t, skuID)
	if qty != 7 || lock != 0 {
		t.Errorf("出库后应为 qty=7 lock=0, got qty=%d lock=%d", qty, lock)
	}
	var logs []jxc.StockLog
	global.GVA_DB.Where("sku_id = ?", skuID).Find(&logs)
	if len(logs) != 1 || logs[0].BusinessType != "sale_out" || logs[0].ChangeQty != -3 || logs[0].AfterQty != 7 {
		t.Errorf("出库流水不符: %+v", logs)
	}
	var order2 jxc.SaleOrder
	global.GVA_DB.First(&order2, order.ID)
	if order2.Status != jxc.SaleStatusShipped {
		t.Errorf("出库后状态应为已出库, got %d", order2.Status)
	}
	// 已出库不可再出库
	if err := saleSvc.ConfirmOut(ctx, order.ID, "tester"); err == nil {
		t.Error("重复出库应拒绝")
	}
}

// TestCancelSaleOrder 测试取消释放锁定
func TestCancelSaleOrder(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := saleSvc.CancelSaleOrder(ctx, order.ID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	_, lock := getStockQty(t, skuID)
	if lock != 0 {
		t.Errorf("取消后锁定应释放, lock=%d", lock)
	}
	var order2 jxc.SaleOrder
	global.GVA_DB.First(&order2, order.ID)
	if order2.Status != jxc.SaleStatusCanceled {
		t.Errorf("状态应为已取消, got %d", order2.Status)
	}
}

// TestDeleteUpdateSaleOrder 测试删除/更新（锁定调整）
func TestDeleteUpdateSaleOrder(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 更新数量 3 → 5（锁 3 → 5）
	upd := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 5, Price: 15}}}
	upd.ID = order.ID
	if err := saleSvc.UpdateSaleOrder(ctx, upd); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	_, lock := getStockQty(t, skuID)
	if lock != 5 {
		t.Errorf("更新后锁定应为 5, got %d", lock)
	}
	// 删除释放锁
	if err := saleSvc.DeleteSaleOrder(ctx, order.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	_, lock = getStockQty(t, skuID)
	if lock != 0 {
		t.Errorf("删除后锁定应释放, got %d", lock)
	}
}

// TestConfirmReturn 测试退货入库
func TestConfirmReturn(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	// 原销售单出库
	sale := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 15}}}
	saleSvc.CreateSaleOrder(ctx, sale)
	saleSvc.ConfirmOut(ctx, sale.ID, "tester")
	// 退货单（关联原单）
	ret := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &sale.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, ret); err != nil {
		t.Fatalf("创建退货单失败: %v", err)
	}
	// 退货单金额应为负（收入减少）
	if ret.TotalAmount >= 0 {
		t.Errorf("退货单总金额应为负值, got %v", ret.TotalAmount)
	}
	if err := saleSvc.ConfirmReturn(ctx, ret.ID, "tester"); err != nil {
		t.Fatalf("退货入库失败: %v", err)
	}
	qty, _ := getStockQty(t, skuID)
	if qty != 8 {
		t.Errorf("退货后库存应为 8, got %d", qty)
	}
	var logs []jxc.StockLog
	global.GVA_DB.Where("sku_id = ? AND business_type = ?", skuID, "sale_return").Find(&logs)
	if len(logs) != 1 || logs[0].ChangeQty != 1 {
		t.Errorf("退货流水不符: %+v", logs)
	}
	// 虚增拦截：草稿阶段各自校验通过（只统计已确认占用），确认时硬校验拒绝超限
	retA := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &sale.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 2, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, retA); err != nil {
		t.Fatalf("创建草稿A失败: %v", err)
	}
	retB := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &sale.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, retB); err != nil {
		t.Fatalf("创建草稿B失败: %v", err)
	}
	if err := saleSvc.ConfirmReturn(ctx, retA.ID, "tester"); err != nil {
		t.Fatalf("草稿A确认失败: %v", err)
	}
	if err := saleSvc.ConfirmReturn(ctx, retB.ID, "tester"); err == nil {
		t.Error("确认时入库超限应被拒绝（禁止虚增）")
	}
	// 非退货单不可退货
	if err := saleSvc.ConfirmReturn(ctx, sale.ID, "tester"); err == nil {
		t.Error("正常销售单执行退货应拒绝")
	}
}

// TestConfirmExchange 测试换货合并单（负明细换出 + 正明细换入, 一次确认）
func TestConfirmExchange(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	sale := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 2, Price: 15}}}
	saleSvc.CreateSaleOrder(ctx, sale)
	saleSvc.ConfirmOut(ctx, sale.ID, "tester")

	// 换货单：换出 1 件（direction=1, 金额正）+ 换入 2 件（direction=2, 金额负）
	ex := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &sale.ID,
		Items: []jxc.SaleItem{
			{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, // 换出
			{SkuID: skuID, Qty: 2, Price: 15, Direction: 2}, // 换入
		}}
	if err := saleSvc.CreateSaleOrder(ctx, ex); err != nil {
		t.Fatalf("创建换货单失败: %v", err)
	}
	// 换货单总金额 = 出 − 入 = 15 − 30 = -15（负向流水, 多退少补）
	if ex.TotalAmount != -15 {
		t.Errorf("换货单总金额应为 -15（15-30）, got %v", ex.TotalAmount)
	}
	// 一次确认同时完成换出与换入
	if err := saleSvc.ConfirmExchange(ctx, ex.ID, "tester"); err != nil {
		t.Fatalf("换货确认失败: %v", err)
	}
	qty, _ := getStockQty(t, skuID)
	if qty != 9 { // 10(初始) - 2(原单出库) - 1(换出) + 2(换入)
		t.Errorf("换货后库存应为 9, got %d", qty)
	}
	var logs []jxc.StockLog
	global.GVA_DB.Where("sku_id = ? AND business_type LIKE 'sale_exchange%'", skuID).Order("id").Find(&logs)
	if len(logs) != 2 || logs[0].BusinessType != "sale_exchange_out" || logs[0].ChangeQty != -1 ||
		logs[1].BusinessType != "sale_exchange_in" || logs[1].ChangeQty != 2 {
		t.Errorf("换货流水不符: %+v", logs)
	}
	// 非换货单不可执行换货
	if err := saleSvc.ConfirmExchange(ctx, sale.ID, "tester"); err == nil {
		t.Error("正常销售单执行换货应拒绝")
	}
	// 换货单可作为原单（换出的商品可继续退/换），但换入数量不可超过原单换出数量
	var g jxc.Goods
	global.GVA_DB.First(&g)
	var sku2 jxc.GoodsSku
	sku2 = jxc.GoodsSku{GoodsID: g.ID, SkuCode: "SP002-M-蓝", Color: "蓝", Size: "M", SalePrice: 15}
	global.GVA_DB.Create(&sku2)
	ex3 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &ex.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: sku2.ID, Qty: 1, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, ex3); err != nil {
		t.Errorf("换货关联换货单应允许: %v", err)
	}
	ex3Over := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &ex.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: sku2.ID, Qty: 2, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, ex3Over); err == nil {
		t.Error("换货关联换货单换入超限应拒绝")
	}
	// 换入明细且无库存记录（创建新库存）；换出 SKU 保留 1 件库存
	global.GVA_DB.Exec("DELETE FROM stock")
	global.GVA_DB.Create(&jxc.Stock{WarehouseID: 1, SkuID: skuID, Quantity: 1})
	ex2 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &ex.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: sku2.ID, Qty: 1, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, ex2); err != nil {
		t.Fatalf("创建换货单失败: %v", err)
	}
	if err := saleSvc.ConfirmExchange(ctx, ex2.ID, "tester"); err != nil {
		t.Fatalf("换货换入失败: %v", err)
	}
	qty, _ = getStockQty(t, sku2.ID)
	if qty != 1 {
		t.Errorf("换入新建库存应为 1, got %d", qty)
	}
	// 允许换不同商品（件数额度）：换入完全不同的商品创建成功（业界允许换不同款，差价多退少补）
	var g2 jxc.Goods
	g2 = jxc.Goods{Code: "SP003", Name: "另一商品", Status: 1}
	global.GVA_DB.Create(&g2)
	var sku3 jxc.GoodsSku
	sku3 = jxc.GoodsSku{GoodsID: g2.ID, SkuCode: "SP003-L-黑", Color: "黑", Size: "L", CostPrice: 10, SalePrice: 20, Status: 1}
	global.GVA_DB.Create(&sku3)
	ex4 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &ex2.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: sku3.ID, Qty: 1, Price: 20, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, ex4); err != nil {
		t.Errorf("换入不同商品应允许（件数额度）: %v", err)
	}
}

// TestSale_StockErrors 测试库存异常场景（出库不足/解锁失败/退货表缺失）
func TestSale_StockErrors(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()
	// 出库时库存不足（建单锁定后被手动改小）
	o1 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 2, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, o1); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	global.GVA_DB.Exec("UPDATE stock SET quantity = 1 WHERE sku_id = ?", skuID)
	if err := saleSvc.ConfirmOut(ctx, o1.ID, "t"); err == nil {
		t.Error("库存不足出库应报错")
	}
	// 删除时库存记录缺失（解锁失败）
	global.GVA_DB.Exec("DELETE FROM stock")
	global.GVA_DB.Create(&jxc.Stock{WarehouseID: 1, SkuID: skuID, Quantity: 10})
	o2 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, o2); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	global.GVA_DB.Exec("DELETE FROM stock")
	if err := saleSvc.DeleteSaleOrder(ctx, o2.ID); err == nil {
		t.Error("库存记录缺失删除应报错")
	}
	// 退货时库存表缺失
	global.GVA_DB.Create(&jxc.Stock{WarehouseID: 1, SkuID: skuID, Quantity: 10})
	// 先建一张已出库的正常销售单作为退货原单
	o0 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, o0); err != nil {
		t.Fatalf("创建原单失败: %v", err)
	}
	if err := saleSvc.ConfirmOut(ctx, o0.ID, "t"); err != nil {
		t.Fatalf("原单出库失败: %v", err)
	}
	ret := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &o0.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, ret); err != nil {
		t.Fatalf("创建退货单失败: %v", err)
	}
	global.GVA_DB.Exec("DROP TABLE stock")
	if err := saleSvc.ConfirmReturn(ctx, ret.ID, "t"); err == nil {
		t.Error("库存表缺失退货应报错")
	}
	// 更新后重新锁定失败（可售库存不足）
	saleTestDB(t)
	skuID = newSaleFixture(t)
	o3 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, o3); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	global.GVA_DB.Exec("UPDATE stock SET quantity = 0 WHERE sku_id = ?", skuID)
	upd3 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 5, Price: 15}}}
	upd3.ID = o3.ID
	if err := saleSvc.UpdateSaleOrder(ctx, upd3); err == nil {
		t.Error("重新锁定库存不足应报错")
	}
}

// TestGetSalePage_Empty 测试空库分页提前返回
func TestGetSalePage_Empty(t *testing.T) {
	saleTestDB(t)
	_, total, err := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}})
	if err != nil || total != 0 {
		t.Errorf("空库应返回 0, got total=%d err=%v", total, err)
	}
}

// TestGetSalePage_Detail 测试分页/详情
func TestGetSalePage_Detail(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	list, total, err := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("分页不符: total=%d list=%d err=%v", total, len(list), err)
	}
	// 按单号搜索
	if _, total, err := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10, Keyword: order.OrderNo}}); err != nil || total != 1 {
		t.Fatalf("按单号搜索不符: total=%d err=%v", total, err)
	}
	detail, err := saleSvc.GetSaleDetail(context.Background(), order.ID)
	if err != nil || len(detail.Items) != 1 {
		t.Fatalf("详情不符: %+v err=%v", detail.Items, err)
	}
	// 剩余可退换件数：原单出库后剩余 = 出库量；退货确认后剩余减少
	if err := saleSvc.ConfirmOut(context.Background(), order.ID, "t"); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	rem, err := saleSvc.GetRemaining(context.Background(), order.ID)
	if err != nil || rem.Remaining != 1 {
		t.Fatalf("剩余件数不符: %+v err=%v", rem, err)
	}
	ret := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(context.Background(), ret); err != nil {
		t.Fatalf("创建退货单失败: %v", err)
	}
	retDetail, err := saleSvc.GetSaleDetail(context.Background(), ret.ID)
	if err != nil || retDetail.Original == nil || retDetail.Original.OrderNo != order.OrderNo {
		t.Fatalf("退货单详情未预加载原单: orig=%+v err=%v", retDetail.Original, err)
	}
	if err := saleSvc.ConfirmReturn(context.Background(), ret.ID, "t"); err != nil {
		t.Fatalf("退货失败: %v", err)
	}
	rem, err = saleSvc.GetRemaining(context.Background(), order.ID)
	if err != nil || rem.Remaining != 0 || rem.UsedQty != 1 {
		t.Fatalf("退货后剩余件数不符: %+v err=%v", rem, err)
	}
	// 原单不存在
	if _, err := saleSvc.GetRemaining(context.Background(), 99999); err == nil {
		t.Error("原单不存在应报错")
	}
}

// TestIsDuplicateKey 单号唯一冲突识别
func TestIsDuplicateKey(t *testing.T) {
	if isDuplicateKey(nil) {
		t.Error("nil 不应判定为重复键")
	}
	if !isDuplicateKey(errors.New("Duplicate entry 'SO-1' for key 'sale_order.uk_sale_order_no'")) {
		t.Error("MySQL 重复键应识别")
	}
	if !isDuplicateKey(errors.New("UNIQUE constraint failed: sale_order.order_no")) {
		t.Error("SQLite 重复键应识别")
	}
	if isDuplicateKey(errors.New("other error")) {
		t.Error("其他错误不应判定为重复键")
	}
}

// TestSale_ErrorBranches 测试销售 service 异常/错误分支
func TestSale_ErrorBranches(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	ctx := context.Background()

	// 无明细 / 数量0 / 负单价 / SKU不存在
	if err := saleSvc.CreateSaleOrder(ctx, &jxc.SaleOrder{WarehouseID: 1}); err == nil {
		t.Error("无明细应报错")
	}
	if err := saleSvc.CreateSaleOrder(ctx, &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 0}}}); err == nil {
		t.Error("数量0应报错")
	}
	if err := saleSvc.CreateSaleOrder(ctx, &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: 999, Qty: 1}}}); err == nil {
		t.Error("SKU不存在应报错")
	}
	// 更新/删除/取消不存在的单
	upd := &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1}}}
	upd.ID = 999
	if err := saleSvc.UpdateSaleOrder(ctx, upd); err == nil {
		t.Error("更新不存在单应报错")
	}
	if err := saleSvc.DeleteSaleOrder(ctx, 999); err == nil {
		t.Error("删除不存在单应报错")
	}
	if err := saleSvc.CancelSaleOrder(ctx, 999); err == nil {
		t.Error("取消不存在单应报错")
	}
	// 正常建单 → 出库 → 已出库不可改/删/取消
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := saleSvc.ConfirmOut(ctx, order.ID, "t"); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	if err := saleSvc.UpdateSaleOrder(ctx, upd); err == nil {
		t.Error("已出库单不应可更新")
	}
	if err := saleSvc.DeleteSaleOrder(ctx, order.ID); err == nil {
		t.Error("已出库单不应可删除")
	}
	if err := saleSvc.CancelSaleOrder(ctx, order.ID); err == nil {
		t.Error("已出库单不应可取消")
	}
	// 出库时库存不足（库存已扣减后再建单锁不住）
	order2 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 10, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order2); err == nil {
		t.Error("可售库存不足应报错")
	}
	// 已出库单不可更新（正确 ID）
	order3 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 3, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order3); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := saleSvc.ConfirmOut(ctx, order3.ID, "t"); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	upd3 := &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	upd3.ID = order3.ID
	if err := saleSvc.UpdateSaleOrder(ctx, upd3); err == nil {
		t.Error("已出库单不应可更新")
	}
	// 库存记录缺失：出库/取消报错
	order4 := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, order4); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	global.GVA_DB.Exec("DELETE FROM stock")
	if err := saleSvc.ConfirmOut(ctx, order4.ID, "t"); err == nil {
		t.Error("库存记录缺失出库应报错")
	}
	if err := saleSvc.CancelSaleOrder(ctx, order4.ID); err == nil {
		t.Error("库存记录缺失取消应报错")
	}
	// 退货：非退货单 / 非待出库退货
	ret := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, ret); err != nil {
		t.Fatalf("创建退货单失败: %v", err)
	}
	if err := saleSvc.ConfirmReturn(ctx, order.ID, "t"); err == nil {
		t.Error("正常销售单执行退货应拒绝")
	}
	// 原单校验：退货关联未出库原单 → 拒绝
	badRet := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order4.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, badRet); err == nil {
		t.Error("退货关联未出库原单应拒绝")
	}
	// 原单校验：原单不存在 → 拒绝
	ghostID := uint(9999)
	ghostRet := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &ghostID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, ghostRet); err == nil {
		t.Error("退货关联不存在原单应拒绝")
	}
	// 换货明细缺方向 → 拒绝
	noDir := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, noDir); err == nil {
		t.Error("换货明细缺方向应拒绝")
	}
	// 换货可关联已退货原单（合法）
	global.GVA_DB.Create(&jxc.Stock{WarehouseID: 1, SkuID: skuID, Quantity: 10})
	if err := saleSvc.ConfirmReturn(ctx, ret.ID, "t"); err != nil {
		t.Fatalf("退货确认失败: %v", err)
	}
	// 换货关联退货单 → 拒绝（退货后客户无货可换）
	exRet := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &ret.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: skuID, Qty: 1, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, exRet); err == nil {
		t.Error("换货关联退货单应拒绝")
	}
	// 换货单只有单边明细 → 拒绝
	oneSide := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &order3.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}}}
	if err := saleSvc.CreateSaleOrder(ctx, oneSide); err == nil {
		t.Error("换货单只有换出明细应拒绝")
	}
	// 换货：换出明细库存不足（清空库存记录）
	global.GVA_DB.Exec("DELETE FROM stock")
	// 方向缺失（绕过创建校验直接插入）：确认应报错
	badDir := &jxc.SaleOrder{OrderNo: "SO-BADDIR", WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &order3.ID, Status: 1}
	global.GVA_DB.Create(badDir)
	global.GVA_DB.Create(&jxc.SaleItem{SaleID: badDir.ID, SkuID: skuID, Qty: 1, Price: 15, Direction: 0})
	if err := saleSvc.ConfirmExchange(ctx, badDir.ID, "t"); err == nil {
		t.Error("方向缺失换货确认应报错")
	}
	exOut := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &order3.ID,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: skuID, Qty: 1, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, exOut); err != nil {
		t.Fatalf("创建换货单失败: %v", err)
	}
	if err := saleSvc.ConfirmExchange(ctx, exOut.ID, "t"); err == nil {
		t.Error("换出库存不足应报错")
	}
	// 更新无明细
	upd2 := &jxc.SaleOrder{WarehouseID: 1}
	upd2.ID = 1
	if err := saleSvc.UpdateSaleOrder(ctx, upd2); err == nil {
		t.Error("更新无明细应报错")
	}
	// 退货/换货重复确认（非待出库拒绝）
	global.GVA_DB.Exec("DELETE FROM stock")
	global.GVA_DB.Create(&jxc.Stock{WarehouseID: 1, SkuID: skuID, Quantity: 5})
	okRet := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, okRet); err != nil {
		t.Fatalf("创建退货单失败: %v", err)
	}
	if err := saleSvc.ConfirmReturn(ctx, okRet.ID, "t"); err != nil {
		t.Fatalf("退货入库失败: %v", err)
	}
	if err := saleSvc.ConfirmReturn(ctx, okRet.ID, "t"); err == nil {
		t.Error("重复退货应拒绝")
	}
	okEx := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeExchange, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15, Direction: 1}, {SkuID: skuID, Qty: 1, Price: 15, Direction: 2}}}
	if err := saleSvc.CreateSaleOrder(ctx, okEx); err != nil {
		t.Fatalf("创建换货单失败: %v", err)
	}
	if err := saleSvc.ConfirmExchange(ctx, okEx.ID, "t"); err != nil {
		t.Fatalf("换货确认失败: %v", err)
	}
	if err := saleSvc.ConfirmExchange(ctx, okEx.ID, "t"); err == nil {
		t.Error("重复换货应拒绝")
	}
	// 退货明细含原单外 SKU → 拒绝（退货限定原单商品）
	var skuB jxc.GoodsSku
	var gTmp jxc.Goods
	global.GVA_DB.First(&gTmp)
	skuB = jxc.GoodsSku{GoodsID: gTmp.ID, SkuCode: "SP001-L-黑", Color: "黑", Size: "L", CostPrice: 8, SalePrice: 15, Status: 1}
	global.GVA_DB.Create(&skuB)
	badSkuRet := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuB.ID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, badSkuRet); err == nil {
		t.Error("退货明细含原单外 SKU 应拒绝")
	}
	// 软删除后重建：单号不重复（Unscoped 查询）
	del1 := &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 1}}}
	if err := saleSvc.CreateSaleOrder(ctx, del1); err != nil {
		t.Fatalf("创建待删单失败: %v", err)
	}
	if err := saleSvc.DeleteSaleOrder(ctx, del1.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	del2 := &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 1}}}
	if err := saleSvc.CreateSaleOrder(ctx, del2); err != nil {
		t.Errorf("删除后重建应成功: %v", err)
	}
	if del2.OrderNo == del1.OrderNo {
		t.Error("重建单号不应与已删除单重复")
	}
	// 单号序号解析失败（坏单号插入后再创建，须在 DROP 之前）
	badDate := time.Now().Format("20060102")
	global.GVA_DB.Create(&jxc.SaleOrder{OrderNo: "SO-" + badDate + "-XXX", WarehouseID: 1, Status: 1})
	if err := saleSvc.CreateSaleOrder(ctx, &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 1}}}); err == nil {
		t.Error("单号解析失败应报错")
	}
	// 明细表缺失：GetRemaining 额度聚合报错
	global.GVA_DB.Exec("DROP TABLE sale_item")
	if _, err := saleSvc.GetRemaining(ctx, order3.ID); err == nil {
		t.Error("明细表缺失 GetRemaining 应报错")
	}
	// 明细表缺失：退货/换货额度校验报错
	retNoItem := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeReturn, OriginalOrderID: &order3.ID, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(ctx, retNoItem); err == nil {
		t.Error("明细表缺失额度校验应报错")
	}
	// 主表缺失时报错
	global.GVA_DB.Exec("DROP TABLE sale_order")
	if err := saleSvc.CreateSaleOrder(ctx, &jxc.SaleOrder{WarehouseID: 1, Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1}}}); err == nil {
		t.Error("主表缺失创建应报错")
	}
	if err := saleSvc.ConfirmOut(ctx, 1, "t"); err == nil {
		t.Error("主表缺失出库应报错")
	}
	if err := saleSvc.ConfirmReturn(ctx, 1, "t"); err == nil {
		t.Error("主表缺失退货应报错")
	}
	if err := saleSvc.ConfirmExchange(ctx, 1, "t"); err == nil {
		t.Error("主表缺失换货应报错")
	}
	// 明细表缺失：GetRemaining / 创建退货（额度聚合）应报错
	global.GVA_DB.Exec("DROP TABLE sale_item")
	if _, err := saleSvc.GetRemaining(ctx, 1); err == nil {
		t.Error("明细表缺失 GetRemaining 应报错")
	}
}
// TestGetSalePage_Range 日期范围过滤（含当天，供看板历史统计下钻）
func TestGetSalePage_Range(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// 当天范围命中
	if _, total, err := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}, From: today, To: today}); err != nil || total != 1 {
		t.Errorf("当天范围应命中 1 单, got total=%d err=%v", total, err)
	}
	// 昨天范围不命中
	if _, total, _ := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}, From: yesterday, To: yesterday}); total != 0 {
		t.Errorf("昨天范围应命中 0 单, got %d", total)
	}
	// 跨范围命中（To 含当天：右区间开区间）
	if _, total, _ := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}, From: yesterday, To: today}); total != 1 {
		t.Errorf("跨范围应命中 1 单, got %d", total)
	}
	// 仅 From 不设 To：从昨天起全部命中
	if _, total, _ := saleSvc.GetSalePage(context.Background(), jxc.SaleQuery{PageInfo: request.PageInfo{Page: 1, PageSize: 10}, From: yesterday}); total != 1 {
		t.Errorf("仅 From 应命中 1 单, got %d", total)
	}
}

// TestSalesHistory_DateFormat 历史统计日期应为 yyyy-mm-dd（防御 MySQL RFC3339 截断）
func TestSalesHistory_DateFormat(t *testing.T) {
	saleTestDB(t)
	skuID := newSaleFixture(t)
	order := &jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal,
		Items: []jxc.SaleItem{{SkuID: skuID, Qty: 1, Price: 15}}}
	if err := saleSvc.CreateSaleOrder(context.Background(), order); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 出库后（status=2）才进入销售统计口径
	if err := saleSvc.ConfirmOut(context.Background(), order.ID, "t"); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	ctx := context.Background()
	dashSvc := &DashboardService{}
	today := time.Now().Format("2006-01-02")

	list, err := dashSvc.SalesHistory(ctx, today, today, "day")
	if err != nil || len(list) != 1 {
		t.Fatalf("当天应有 1 条统计: %+v err=%v", list, err)
	}
	for _, item := range list {
		if len(item.Date) != 10 || strings.Contains(item.Date, "T") {
			t.Errorf("日期应为 yyyy-mm-dd 纯日期, got %q", item.Date)
		}
	}
}
