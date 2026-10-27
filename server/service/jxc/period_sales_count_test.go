package jxc

import (
	"context"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 月报单数口径：salesOrders 是已出库单数（含已红冲），salesOrdersReversed 单列红冲数；
// 金额/成本已净额。前端用两个字段做“N 单（含 M 张已红冲）”，不再二次扣减。
func TestPeriod_SalesOrdersReversed(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 5, 10)
	ss := NewSaleService()
	rvs := NewReversalService()
	periods := NewPeriodService()
	ctx := context.Background()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("开销售单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	if _, err := rvs.Reverse(ctx, jxc.ReversalSourceSale, so.ID, "录错", "t"); err != nil {
		t.Fatalf("红冲失败: %v", err)
	}

	sum, err := periods.Summary(ctx, time.Now().In(jxcLocation).Format("2006-01"))
	if err != nil {
		t.Fatalf("月报失败: %v", err)
	}
	if sum.SalesOrders != 1 || sum.SalesOrdersReversed != 1 {
		t.Errorf("单数应为 1（含红冲 1），实际 %d/%d", sum.SalesOrders, sum.SalesOrdersReversed)
	}
	if sum.SalesAmount != 0 || sum.SalesCost != 0 || sum.Gross != 0 {
		t.Errorf("今卖今红后金额/成本/毛利应归零，实际 %.2f/%.2f/%.2f",
			sum.SalesAmount, sum.SalesCost, sum.Gross)
	}
}
