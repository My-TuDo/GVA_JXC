package jxc

import (
	"context"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 红冲后的采购退货不能再冲减采购额：月报/工作台都必须忽略 reversed_at 非空的行。
func TestPurchaseReturnReversal_ExcludedFromAggregates(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps := NewPurchaseService()
	prs := NewPurchaseReturnService()
	rvs := NewReversalService()
	periods := NewPeriodService()
	ds := NewDashboardService()
	ctx := context.Background()
	now := time.Now().In(jxcLocation)

	po, err := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 5}}})
	if err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}
	_ = ps.Approve(ctx, po.ID, "t")
	if err := ps.Receive(ctx, po.ID, "t", now); err != nil {
		t.Fatalf("采购入库失败: %v", err)
	}
	pr, err := prs.Create(ctx, &jxc.PurchaseReturn{PurchaseID: po.ID,
		Items: []jxc.PurchaseReturnItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("建采购退货失败: %v", err)
	}
	if err := prs.Post(ctx, pr.ID, "t", now); err != nil {
		t.Fatalf("采购退货出库失败: %v", err)
	}
	if _, err := rvs.Reverse(ctx, jxc.ReversalSourcePurchaseReturn, pr.ID, "取消退货", "t"); err != nil {
		t.Fatalf("采购退货红冲失败: %v", err)
	}

	month := now.Format("2006-01")
	sum, err := periods.Summary(ctx, month)
	if err != nil {
		t.Fatalf("月报失败: %v", err)
	}
	if sum.PurchaseAmount != 50 || sum.PurchaseReturnAmount != 0 || sum.StockValue != 50 {
		t.Errorf("红冲后退货不应冲减采购额：采购 %.2f / 退货 %.2f / 期末 %.2f",
			sum.PurchaseAmount, sum.PurchaseReturnAmount, sum.StockValue)
	}

	dash, err := ds.Summary(ctx, now.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("工作台失败: %v", err)
	}
	if dash.TodayPurchaseAmount != 50 || dash.StockValue != 50 {
		t.Errorf("工作台也应忽略已红冲退货：净采购 %.2f / 库存 %.2f",
			dash.TodayPurchaseAmount, dash.StockValue)
	}
}
