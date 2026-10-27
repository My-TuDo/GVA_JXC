package jxc

import (
	"context"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 工作台必须把当日红冲计入负数业务：今卖今红，销售额/毛利归零，库存回补。
func TestDashboard_ReversalNetsToday(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 5, 10)
	ss := NewSaleService()
	rs := NewReversalService()
	ds := NewDashboardService()
	ctx := context.Background()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("开销售单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	if _, err := rs.Reverse(ctx, jxc.ReversalSourceSale, so.ID, "录错", "t"); err != nil {
		t.Fatalf("红冲失败: %v", err)
	}

	got, err := ds.Summary(ctx, time.Now().In(jxcLocation).Format("2006-01-02"))
	if err != nil {
		t.Fatalf("工作台汇总失败: %v", err)
	}
	if got.TodaySalesAmount != 0 || got.TodayGross != 0 {
		t.Errorf("今卖今红后净销售/毛利应为 0，实际 %.2f/%.2f", got.TodaySalesAmount, got.TodayGross)
	}
	// 当前口径：订单数仍算“当日已出库（含已红冲）”；金额/成本已净额。
	// 净单数是否需要扣红冲待产品确认，见 docs/30 第八节。
	if got.TodaySalesOrders != 1 || got.TodaySalesOrdersReversed != 1 {
		t.Errorf("当前口径下今日销售单数应为 1（其中红冲 1），实际 %d/%d",
			got.TodaySalesOrders, got.TodaySalesOrdersReversed)
	}
	if got.StockValue != 50 {
		t.Errorf("红冲后库存金额应回到 50，实际 %.2f", got.StockValue)
	}
}

// 工作台净采购必须扣当日采购退货。
func TestDashboard_PurchaseReturnNetsToday(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps := NewPurchaseService()
	prs := NewPurchaseReturnService()
	ds := NewDashboardService()
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
	ret, err := prs.Create(ctx, &jxc.PurchaseReturn{PurchaseID: po.ID,
		Items: []jxc.PurchaseReturnItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("建采购退货失败: %v", err)
	}
	if err := prs.Post(ctx, ret.ID, "t", time.Now()); err != nil {
		t.Fatalf("采购退货出库失败: %v", err)
	}

	got, err := ds.Summary(ctx, time.Now().In(jxcLocation).Format("2006-01-02"))
	if err != nil {
		t.Fatalf("工作台汇总失败: %v", err)
	}
	if got.TodayPurchaseAmount != 40 {
		t.Errorf("今日净采购应为 50-10=40，实际 %.2f", got.TodayPurchaseAmount)
	}
	if got.StockValue != 40 {
		t.Errorf("退货后库存金额应为 40，实际 %.2f", got.StockValue)
	}
}
