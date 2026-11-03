package jxc

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/internal/money"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// 本文件是接手审计过程中定位到的真实缺陷的回归测试。
// 每一条在修复前都必须先红，修复后保持绿；测试名直接描述业务后果。

// TestReversal_SaleWithPostedReturnRejected
// 已退货的销售单不能再整单红冲：否则退货回补 1 件 + 红冲回补 4 件 = 4 件的单回了 5 件。
func TestReversal_SaleWithPostedReturnRejected(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 10, 88)
	_, ss, rs, rts, ls := servicesAll()
	ctx := context.Background()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 4, Price: 199}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	ret, err := rts.Create(ctx, &jxc.SaleReturn{SaleID: so.ID,
		Items: []jxc.SaleReturnItem{{SkuID: sku, Qty: 1}}, Reason: "尺码不合适"})
	if err != nil {
		t.Fatalf("建退货单失败: %v", err)
	}
	if err := rts.Post(ctx, ret.ID, "t", time.Now()); err != nil {
		t.Fatalf("退货入库失败: %v", err)
	}

	if _, err := rs.Reverse(ctx, jxc.ReversalSourceSale, so.ID, "整单作废", "t"); err == nil {
		t.Fatal("已发生退货的销售单不应允许整单红冲（会把库存冲多）")
	} else if !strings.Contains(err.Error(), "退货") {
		t.Fatalf("错误信息应说明存在退货，got %v", err)
	}
	if q, v := balanceOf(t, db, wh, sku); q != 7 || v != money.FromYuan(616) {
		t.Fatalf("拒绝红冲后应为 7/616.00，实际 %d/%s", q, v)
	}
	mustInvariant(t, ls)
}

// TestReturn_DraftsCannotOverReturn
// 两张各自合法的草稿退货单，先后入库时第二张必须被拒绝：
// 校验只在建单时做，等于允许"先建两张、再依次入库"绕过可退上限。
func TestReturn_DraftsCannotOverReturn(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 2, 50)
	_, ss, _, rts, ls := servicesAll()
	ctx := context.Background()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2, Price: 99}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}

	mk := func() *jxc.SaleReturn {
		ret, err := rts.Create(ctx, &jxc.SaleReturn{SaleID: so.ID,
			Items: []jxc.SaleReturnItem{{SkuID: sku, Qty: 2}}})
		if err != nil {
			t.Fatalf("建退货单失败: %v", err)
		}
		return ret
	}
	r1, r2 := mk(), mk()
	if err := rts.Post(ctx, r1.ID, "t", time.Now()); err != nil {
		t.Fatalf("第一张退货入库失败: %v", err)
	}
	if err := rts.Post(ctx, r2.ID, "t", time.Now()); err == nil {
		t.Fatal("第二张退货已超过可退上限，入库必须被拒绝")
	} else if !strings.Contains(err.Error(), "最多可退") && !strings.Contains(err.Error(), "超过") {
		t.Fatalf("错误信息应说明可退上限，got %v", err)
	}
	if q, _ := balanceOf(t, db, wh, sku); q != 2 {
		t.Fatalf("只应回补 2 件，实际结存 %d", q)
	}
	mustInvariant(t, ls)
}

// TestReversal_PurchaseUsesOriginalCost
// 采购红冲必须按原单价冲回。用当前移动均价冲，会把剩余批次的成本改写。
// 10@5 + 10@7 后冲掉第一张：正确结果是 10 件 / 70 元，而不是 10 件 / 60 元。
func TestReversal_PurchaseUsesOriginalCost(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps, _, rs, _, ls := servicesAll()
	ctx := context.Background()

	po1, _ := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 5}}})
	_ = ps.Approve(ctx, po1.ID, "t")
	if err := ps.Receive(ctx, po1.ID, "t", time.Now()); err != nil {
		t.Fatalf("第一批入库失败: %v", err)
	}
	po2, _ := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 7}}})
	_ = ps.Approve(ctx, po2.ID, "t")
	if err := ps.Receive(ctx, po2.ID, "t", time.Now()); err != nil {
		t.Fatalf("第二批入库失败: %v", err)
	}

	if _, err := rs.Reverse(ctx, jxc.ReversalSourcePurchase, po1.ID, "供应商开错票", "t"); err != nil {
		t.Fatalf("红冲失败: %v", err)
	}
	if q, v := balanceOf(t, db, wh, sku); q != 10 || v != money.FromYuan(70) {
		t.Fatalf("按原价冲回后应为 10/70.00，实际 %d/%s", q, v)
	}
	mustInvariant(t, ls)
}

// TestSale_PriceComesFromSku
// 销售单价必须取 SKU 销售价，客户端传入的 price 不能生效（改价权限未开放）。
func TestSale_PriceComesFromSku(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db) // salePrice = 99
	seedStock(t, db, wh, sku, 5, 10)
	_, ss, _, _, ls := servicesAll()
	ctx := context.Background()

	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2, Price: 1}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if so.TotalAmount != 198 {
		t.Errorf("总额应按 SKU 售价 99×2=198，实际 %.2f", so.TotalAmount)
	}
	var item jxc.SaleItem
	if err := db.Where("sale_id = ?", so.ID).First(&item).Error; err != nil {
		t.Fatalf("查明细失败: %v", err)
	}
	if item.Price != 99 {
		t.Errorf("明细单价应为 SKU 售价 99，实际 %.2f", item.Price)
	}
	_ = ss.Ship(ctx, so.ID, "t", time.Now())
	mustInvariant(t, ls)
}

// TestSale_ConcurrentCreateOrderNo
// 并发生成单号不能互相撞车：POS 多窗口/多店员同时开单是常态。
func TestSale_ConcurrentCreateOrderNo(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 20, 10)
	_, ss, _, _, ls := servicesAll()
	ctx := context.Background()

	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0, n)
	nos := make([]string, 0, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
				Items: []jxc.SaleItem{{SkuID: sku, Qty: 1, Price: 10}}})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			nos = append(nos, so.OrderNo)
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("%d 个并发开单中 %d 个失败，首个错误：%v", n, len(errs), errs[0])
	}
	seen := map[string]bool{}
	for _, no := range nos {
		if seen[no] {
			t.Fatalf("单号重复：%s", no)
		}
		seen[no] = true
	}
	var cnt int64
	db.Model(&jxc.SaleOrder{}).Count(&cnt)
	if cnt != n {
		t.Errorf("应落库 %d 张销售单，实际 %d", n, cnt)
	}
	mustInvariant(t, ls)
}

// TestStockCheck_OpeningBalanceCost
// 期初建账走盘盈时，必须能把成本一起录进来；否则库存有数量、金额为 0，
// 卖出去就是 100% 毛利（这是"期初建账"路径最容易埋的坑）。
func TestStockCheck_OpeningBalanceCost(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	cs := NewStockCheckService()
	ctx := context.Background()

	check, err := cs.Create(ctx, wh, "期初建账")
	if err != nil {
		t.Fatalf("建盘点单失败: %v", err)
	}
	var item jxc.StockCheckItem
	if err := db.Where("check_id = ? AND sku_id = ?", check.ID, sku).First(&item).Error; err != nil {
		t.Fatalf("查盘点明细失败: %v", err)
	}
	cost := 60.0
	if err := cs.UpdateItems(ctx, check.ID, []StockCountInput{{ID: item.ID, ActualQty: 5, UnitCost: &cost}}); err != nil {
		t.Fatalf("录入期初失败: %v", err)
	}
	if _, err := cs.Post(ctx, check.ID, "t", time.Now()); err != nil {
		t.Fatalf("完成盘点失败: %v", err)
	}
	if q, v := balanceOf(t, db, wh, sku); q != 5 || v != money.FromYuan(300) {
		t.Fatalf("期初 5 件 @60 应为 5/300.00，实际 %d/%s", q, v)
	}
	mustInvariant(t, new(LedgerService))
}

// TestPeriod_SnapshotIsAsOfMonthEnd
// 10 月结账时，11 月已发生的业务不能混进 10 月快照。
func TestPeriod_SnapshotIsAsOfMonthEnd(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ls := new(LedgerService)
	ps := NewPeriodService()
	ctx := context.Background()

	oct := time.Date(2026, 10, 15, 10, 0, 0, 0, time.Local)
	nov := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	if err := ls.Post(ctx, nil, []MoveInput{{
		WarehouseID: wh, SkuID: sku, Qty: 10, UnitCost: 5,
		SourceType: jxc.MoveSourcePurchaseIn, SourceID: 1, SourceLineID: 1, OccurredAt: oct,
	}}); err != nil {
		t.Fatalf("10 月入库失败: %v", err)
	}
	if err := ls.Post(ctx, nil, []MoveInput{{
		WarehouseID: wh, SkuID: sku, Qty: 10, UnitCost: 7,
		SourceType: jxc.MoveSourcePurchaseIn, SourceID: 2, SourceLineID: 1, OccurredAt: nov,
	}}); err != nil {
		t.Fatalf("11 月入库失败: %v", err)
	}

	period, err := ps.Close(ctx, "2026-10", "t")
	if err != nil {
		t.Fatalf("结账失败: %v", err)
	}
	snaps, err := ps.Snapshot(ctx, period.ID)
	if err != nil {
		t.Fatalf("取快照失败: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Qty != 10 || snaps[0].Value != 50 {
		t.Fatalf("10 月快照应为 10 件/50 元（不应包含 11 月的 10 件/70），实际 %+v", snaps)
	}

	octSum, err := ps.Summary(ctx, "2026-10")
	if err != nil {
		t.Fatalf("10 月报失败: %v", err)
	}
	if octSum.StockValue != 50 {
		t.Errorf("10 月期末库存应为 50，实际 %.2f", octSum.StockValue)
	}
	novSum, err := ps.Summary(ctx, "2026-11")
	if err != nil {
		t.Fatalf("11 月报失败: %v", err)
	}
	if novSum.StockValue != 120 {
		t.Errorf("11 月期末库存应为 120，实际 %.2f", novSum.StockValue)
	}
}

// TestPeriod_ReversalHitsReversalMonth
// 9 月销售、10 月红冲：9 月报表必须保持不变，10 月以负数体现冲减。
func TestPeriod_ReversalHitsReversalMonth(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ls := new(LedgerService)
	_, ss, rs, _, _ := servicesAll()
	ps := NewPeriodService()
	ctx := context.Background()

	sep := time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local)
	if err := ls.Post(ctx, nil, []MoveInput{{
		WarehouseID: wh, SkuID: sku, Qty: 10, UnitCost: 50,
		SourceType: jxc.MoveSourcePurchaseIn, SourceID: 1, SourceLineID: 1, OccurredAt: sep,
	}}); err != nil {
		t.Fatalf("9 月入库失败: %v", err)
	}
	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", sep); err != nil {
		t.Fatalf("9 月出库失败: %v", err)
	}
	oct := time.Date(2026, 10, 15, 10, 0, 0, 0, time.Local)
	if _, err := rs.ReverseAt(ctx, jxc.ReversalSourceSale, so.ID, "录错了", "t", oct); err != nil {
		t.Fatalf("红冲失败: %v", err)
	}

	sepSum, err := ps.Summary(ctx, "2026-09")
	if err != nil {
		t.Fatalf("9 月报失败: %v", err)
	}
	if sepSum.SalesAmount != 198 || sepSum.SalesCost != 100 {
		t.Errorf("9 月销售应为 198/成本 100（红冲属于 10 月），实际 %.2f/%.2f",
			sepSum.SalesAmount, sepSum.SalesCost)
	}
	octSum, err := ps.Summary(ctx, "2026-10")
	if err != nil {
		t.Fatalf("10 月报失败: %v", err)
	}
	if octSum.SalesAmount != -198 || octSum.SalesCost != -100 {
		t.Errorf("10 月应冲减 -198/-100，实际 %.2f/%.2f", octSum.SalesAmount, octSum.SalesCost)
	}
}

// TestPeriod_RejectsFutureClose
func TestPeriod_RejectsFutureClose(t *testing.T) {
	_ = newLedgerTestDB(t)
	ps := NewPeriodService()
	if _, err := ps.Close(context.Background(), "2099-01", "t"); err == nil {
		t.Fatal("结未来的账必须被拒绝")
	}
}

// TestStockCheck_RejectsChangedBookQty
// 盘点建单后账面发生了变化（期间又卖货），完成时必须拒绝，否则会按过期差异调账。
func TestStockCheck_RejectsChangedBookQty(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 10, 50)
	_, ss, _, _, ls := servicesAll()
	cs := NewStockCheckService()
	ctx := context.Background()

	check, err := cs.Create(ctx, wh, "月度盘点")
	if err != nil {
		t.Fatalf("建盘点单失败: %v", err)
	}
	var item jxc.StockCheckItem
	if err := db.Where("check_id = ?", check.ID).First(&item).Error; err != nil {
		t.Fatalf("查盘点明细失败: %v", err)
	}
	if err := cs.UpdateItems(ctx, check.ID, []StockCountInput{{ID: item.ID, ActualQty: 10}}); err != nil {
		t.Fatalf("录入实盘失败: %v", err)
	}

	// 盘点期间又卖了 2 件，账面从 10 变成 8；此时按旧差异（0）完成盘点会把差异吞掉。
	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2, Price: 99}}})
	if err != nil {
		t.Fatalf("开单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}

	if _, err := cs.Post(ctx, check.ID, "t", time.Now()); err == nil {
		t.Fatal("账面数已变化，完成盘点必须被拒绝并要求重新盘点")
	} else if !strings.Contains(err.Error(), "账面") {
		t.Fatalf("错误信息应说明账面已变化，got %v", err)
	}
	if q, _ := balanceOf(t, db, wh, sku); q != 8 {
		t.Fatalf("拒绝后应保持 8 件，实际 %d", q)
	}
	mustInvariant(t, ls)
}
