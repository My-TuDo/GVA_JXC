package jxc

import (
	"context"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 工作台聚合：KPI 必须与账本/单据一致，不能受 pageSize=100 限制；
// 低库存按 available=结存-有效预留 计算。
func TestDashboard_Summary(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps := NewPurchaseService()
	ss := NewSaleService()
	rs := NewReturnService()
	prs := NewPurchaseReturnService()
	ctx := context.Background()

	po, err := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 5}}})
	if err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}
	_ = ps.Approve(ctx, po.ID, "t")
	if err := ps.Receive(ctx, po.ID, "t", time.Now()); err != nil {
		t.Fatalf("采购入库失败: %v", err)
	}

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 3}}})
	if err != nil {
		t.Fatalf("开销售单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("销售出库失败: %v", err)
	}
	ret, err := rs.Create(ctx, &jxc.SaleReturn{SaleID: so.ID,
		Items: []jxc.SaleReturnItem{{SkuID: sku, Qty: 1}}})
	if err != nil {
		t.Fatalf("建退货单失败: %v", err)
	}
	if err := rs.Post(ctx, ret.ID, "t", time.Now()); err != nil {
		t.Fatalf("退货入库失败: %v", err)
	}

	// 一张草稿采购单 + 一张采购退货草稿，验证待办计数。
	if _, err := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 1, UnitCost: 5}}}); err != nil {
		t.Fatalf("建草稿采购单失败: %v", err)
	}
	if _, err := prs.Create(ctx, &jxc.PurchaseReturn{PurchaseID: po.ID,
		Items: []jxc.PurchaseReturnItem{{SkuID: sku, Qty: 1}}}); err != nil {
		t.Fatalf("建采购退货草稿失败: %v", err)
	}

	ds := NewDashboardService()
	today := time.Now().In(jxcLocation).Format("2006-01-02")
	got, err := ds.Summary(ctx, today)
	if err != nil {
		t.Fatalf("工作台汇总失败: %v", err)
	}

	if got.Date != today {
		t.Errorf("date 应为 %s，实际 %s", today, got.Date)
	}
	if got.TodayPurchaseAmount != 50 {
		t.Errorf("今日净采购应为 50，实际 %.2f", got.TodayPurchaseAmount)
	}
	if got.TodaySalesAmount != 198 || got.TodaySalesOrders != 1 {
		t.Errorf("今日净销售应为 198/1 单，实际 %.2f/%d", got.TodaySalesAmount, got.TodaySalesOrders)
	}
	if got.TodayReturnAmount != 99 || got.TodayReturnOrders != 1 {
		t.Errorf("今日退货应为 99/1 单，实际 %.2f/%d", got.TodayReturnAmount, got.TodayReturnOrders)
	}
	if got.TodayGross != 188 {
		t.Errorf("今日毛利应为 198-(15-5)=188，实际 %.2f", got.TodayGross)
	}
	if got.StockValue != 40 || got.SkuCount != 1 {
		t.Errorf("库存应为 40/1 个 SKU，实际 %.2f/%d", got.StockValue, got.SkuCount)
	}
	if got.Pending.Approve != 1 || got.Pending.PurchaseReturnDraft != 1 || got.Pending.Total != 2 {
		t.Errorf("待办应为 approve=1/prDraft=1/total=2，实际 %+v", got.Pending)
	}
	if got.LowStock.Threshold != 10 || got.LowStock.Count != 1 || len(got.LowStock.Items) != 1 {
		t.Fatalf("低库存应有 1 条，实际 %+v", got.LowStock)
	}
	if got.LowStock.Items[0].Available != 8 || got.LowStock.Items[0].GoodsName == "" {
		t.Errorf("低库存明细应为 available=8 且有商品名，实际 %+v", got.LowStock.Items[0])
	}

	if _, err := ds.Summary(ctx, "2026/10/27"); err == nil {
		t.Error("非法 date 格式应报错")
	}
}
