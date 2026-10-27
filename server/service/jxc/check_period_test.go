package jxc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/internal/money"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 盘点与月结测试。
//
// 盘点要证明的核心命题：**差异进账本，而不是覆盖余额**。
// 月结要证明的是：结账后历史不可再变，且结账前必须先对平账。

func TestStockCheck_DiffGoesToLedger(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 10, 5) // 10 件 @5
	cs := NewStockCheckService()
	ls := new(LedgerService)
	ctx := context.Background()

	check, err := cs.Create(ctx, wh, "月底盘点")
	if err != nil {
		t.Fatalf("建盘点单失败: %v", err)
	}
	if len(check.Items) != 1 || check.Items[0].BookQty != 10 {
		t.Fatalf("盘点明细应快照账面数 10，实际 %+v", check.Items)
	}

	// 实盘 7 件 → 盘亏 3
	if err := cs.UpdateItems(ctx, check.ID, []StockCountInput{{ID: check.Items[0].ID, ActualQty: 7}}); err != nil {
		t.Fatalf("录入实盘失败: %v", err)
	}
	posted, err := cs.Post(ctx, check.ID, "tester", time.Now())
	if err != nil {
		t.Fatalf("完成盘点失败: %v", err)
	}
	if posted.Status != jxc.StockCheckPosted {
		t.Errorf("盘点单状态应为已完成，实际 %d", posted.Status)
	}

	if q, v := balanceOf(t, db, wh, sku); q != 7 || v != money.FromYuan(35) {
		t.Errorf("盘亏后应为 7 件/35 元，实际 %d/%s", q, v)
	}
	// 关键：账本里必须有一条 check_loss，且余额是由它推出来的
	var cnt int64
	db.Model(&jxc.StockMove{}).Where("source_type = ?", jxc.MoveSourceCheckLoss).Count(&cnt)
	if cnt != 1 {
		t.Errorf("应有一条盘亏账本行，实际 %d", cnt)
	}
	mustInvariant(t, ls)
}

// TestStockCheck_GainAndGuards 盘盈 + 各种非法状态
func TestStockCheck_GainAndGuards(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 4, 10)
	cs := NewStockCheckService()
	ctx := context.Background()

	check, err := cs.Create(ctx, wh, "")
	if err != nil {
		t.Fatalf("建单失败: %v", err)
	}
	// 同一仓库不能有第二张进行中的盘点单
	if _, err := cs.Create(ctx, wh, ""); err == nil || !strings.Contains(err.Error(), "进行中") {
		t.Errorf("重复建盘点单应被拒绝，got %v", err)
	}
	// 负数实盘
	if err := cs.UpdateItems(ctx, check.ID, []StockCountInput{{ID: check.Items[0].ID, ActualQty: -1}}); err == nil {
		t.Error("负数实盘应被拒绝")
	}
	// 没录入就完成
	if _, err := cs.Post(ctx, check.ID, "t", time.Now()); err == nil ||
		!strings.Contains(err.Error(), "还没有录入") {
		t.Errorf("未录入实盘不应能完成，got %v", err)
	}

	// 盘盈 2 件
	if err := cs.UpdateItems(ctx, check.ID, []StockCountInput{{ID: check.Items[0].ID, ActualQty: 6}}); err != nil {
		t.Fatalf("录入失败: %v", err)
	}
	if _, err := cs.Post(ctx, check.ID, "t", time.Now()); err != nil {
		t.Fatalf("完成盘点失败: %v", err)
	}
	if q, v := balanceOf(t, db, wh, sku); q != 6 || v != money.FromYuan(60) {
		t.Errorf("盘盈后应为 6 件/60 元，实际 %d/%s", q, v)
	}
	// 已完成的盘点不能再完成/取消
	if _, err := cs.Post(ctx, check.ID, "t", time.Now()); err == nil {
		t.Error("重复完成应被拒绝")
	}
	if err := cs.Cancel(ctx, check.ID); err == nil {
		t.Error("已完成的盘点不应能取消")
	}
}

// TestPeriod_CloseAndFreeze 结账后：快照冻结、期间拒绝写入
func TestPeriod_CloseAndFreeze(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 5, 20)
	ps := NewPeriodService()
	ls := new(LedgerService)
	ctx := context.Background()

	ym := time.Now().In(jxcLocation).Format("2006-01")
	period, err := ps.Close(ctx, ym, "tester")
	if err != nil {
		t.Fatalf("结账失败: %v", err)
	}
	if period.Status != jxc.PeriodClosed || period.ClosedBy != "tester" {
		t.Errorf("期间状态不对：%+v", period)
	}

	// 快照应有 1 条，数量金额与结存一致
	snaps, err := ps.Snapshot(ctx, period.ID)
	if err != nil {
		t.Fatalf("取快照失败: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Qty != 5 || snaps[0].Value != 100 {
		t.Fatalf("快照应为 1 条 5 件 100 元，实际 %+v", snaps)
	}

	// 结账后往这个月过账必须被拒
	err = ls.Post(ctx, nil, []MoveInput{{
		WarehouseID: wh, SkuID: sku, Qty: 1, UnitCost: 20,
		SourceType: jxc.MoveSourcePurchaseIn, SourceID: 999, SourceLineID: 1,
		OccurredAt: time.Now(),
	}})
	if err == nil || !strings.Contains(err.Error(), "已结账") {
		t.Errorf("已结账期间不应能写入，got %v", err)
	}

	// 重复结账
	if _, err := ps.Close(ctx, ym, "tester"); err == nil {
		t.Error("重复结账应被拒绝")
	}
}

// TestPeriod_SummaryCompoundsReturns 月报口径：销售额与成本都要扣掉退货
func TestPeriod_SummaryCompoundsReturns(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 10, 50)
	_, ss, _, rts, _ := servicesAll()
	ps := NewPeriodService()
	ctx := context.Background()
	now := time.Now()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 4, Price: 100}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", now); err != nil {
		t.Fatalf("出库失败: %v", err)
	}

	ret, err := rts.Create(ctx, &jxc.SaleReturn{SaleID: so.ID,
		Items: []jxc.SaleReturnItem{{SkuID: sku, Qty: 1}}})
	if err != nil {
		t.Fatalf("建退货单失败: %v", err)
	}
	if err := rts.Post(ctx, ret.ID, "t", now); err != nil {
		t.Fatalf("退货入库失败: %v", err)
	}

	sum, err := ps.Summary(ctx, now.Format("2006-01"))
	if err != nil {
		t.Fatalf("取月报失败: %v", err)
	}
	// 单价固定取 SKU 售价 99：销售 4×99=396，退 1×99=99 → 净销售额 297
	// 成本 4×50=200，退 1×50=50 → 净成本 150，毛利 147
	if sum.SalesAmount != 297 {
		t.Errorf("净销售额应为 297，实际 %.2f", sum.SalesAmount)
	}
	if sum.SalesCost != 150 {
		t.Errorf("净成本应为 150，实际 %.2f", sum.SalesCost)
	}
	if sum.Gross != 147 {
		t.Errorf("毛利应为 147，实际 %.2f", sum.Gross)
	}
	if sum.ReturnAmount != 99 || sum.ReturnOrders != 1 {
		t.Errorf("退货口径不对：%.2f / %d", sum.ReturnAmount, sum.ReturnOrders)
	}
	_ = db
}
