package jxc

import (
	"context"
	"testing"
	"time"

	common "github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
	jxcReq "github.com/flipped-aurora/gin-vue-admin/server/model/jxc/request"
)

// 单据列表服务端筛选的回归测试。
// 重点：total 是数据库真实数量（不是前端 100 条切片），筛选条件能组合，
// 采购退货的 supplierName / warehouseName / itemCount 能直接给前端用。

func TestDocumentSearch_Purchase(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps := NewPurchaseService()
	ctx := context.Background()

	supplier := jxc.Partner{Kind: jxc.PartnerSupplier, Code: "SUP-1", Name: "布料商", Status: 1}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatalf("建供应商失败: %v", err)
	}
	po1, err := ps.Create(ctx, &jxc.PurchaseOrder{
		WarehouseID: wh, SupplierID: &supplier.ID, Remark: "蓝色面料",
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 3, UnitCost: 5}},
	})
	if err != nil {
		t.Fatalf("建采购单1失败: %v", err)
	}
	po2, err := ps.Create(ctx, &jxc.PurchaseOrder{
		WarehouseID: wh, Remark: "红色面料",
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 7}},
	})
	if err != nil {
		t.Fatalf("建采购单2失败: %v", err)
	}
	if err := ps.Approve(ctx, po2.ID, "t"); err != nil {
		t.Fatalf("审核失败: %v", err)
	}
	if err := ps.Receive(ctx, po2.ID, "t", time.Now()); err != nil {
		t.Fatalf("入库失败: %v", err)
	}

	statusReceived := int8(jxc.PurchaseStatusReceived)
	search := func(info jxcReq.DocumentSearch) ([]jxc.PurchaseOrder, int64) {
		t.Helper()
		list, total, err := ps.Search(ctx, info)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		return list, total
	}

	list, total := search(jxcReq.DocumentSearch{
		PageInfo: common.PageInfo{Page: 1, PageSize: 20}, Status: &statusReceived,
	})
	if total != 1 || len(list) != 1 || list[0].ID != po2.ID {
		t.Fatalf("按已入库筛选应只有 po2，实际 total=%d list=%+v", total, list)
	}

	list, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20, Keyword: "布料商"}})
	if total != 1 || list[0].ID != po1.ID {
		t.Errorf("按供应商名称筛选应命中 po1，实际 total=%d list=%+v", total, list)
	}
	list, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20, Keyword: "红色"}})
	if total != 1 || list[0].ID != po2.ID {
		t.Errorf("按备注筛选应命中 po2，实际 total=%d", total)
	}
	list, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20}, PartnerID: supplier.ID})
	if total != 1 || list[0].ID != po1.ID {
		t.Errorf("按供应商 ID 筛选应命中 po1，实际 total=%d", total)
	}

	// 分页：真实 total=2，两页各 1 条且不重叠
	page1, total := search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 1}})
	if total != 2 || len(page1) != 1 {
		t.Fatalf("第 1 页应为 2 条总数/1 条数据，实际 %d/%d", total, len(page1))
	}
	page2, _ := search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 2, PageSize: 1}})
	if len(page2) != 1 || page1[0].ID == page2[0].ID {
		t.Errorf("第 2 页应返回另一条，实际 %+v / %+v", page1, page2)
	}

	// 日期范围：明天开始应为 0；今天应为 2
	tomorrow := time.Now().In(jxcLocation).AddDate(0, 0, 1).Format("2006-01-02")
	_, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20}, StartAt: tomorrow, EndAt: tomorrow})
	if total != 0 {
		t.Errorf("明天没有单据，实际 total=%d", total)
	}
	today := time.Now().In(jxcLocation).Format("2006-01-02")
	_, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20}, StartAt: today, EndAt: today})
	if total != 2 {
		t.Errorf("今天的单据应有 2 条，实际 %d", total)
	}

	// 红冲后 reversed=true 只能看到 po2
	if _, err := NewReversalService().Reverse(ctx, jxc.ReversalSourcePurchase, po2.ID, "录错", "t"); err != nil {
		t.Fatalf("红冲失败: %v", err)
	}
	reversed := true
	list, total = search(jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20}, Reversed: &reversed})
	if total != 1 || list[0].ID != po2.ID {
		t.Errorf("reversed=true 应只命中 po2，实际 total=%d list=%+v", total, list)
	}
}

func TestDocumentSearch_Sale(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	seedStock(t, db, wh, sku, 10, 5)
	ss := NewSaleService()
	ctx := context.Background()

	draft, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 1}}})
	if err != nil {
		t.Fatalf("建草稿销售单失败: %v", err)
	}
	shipped, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh,
		Items: []jxc.SaleItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("建销售单失败: %v", err)
	}
	if err := ss.Ship(ctx, shipped.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}

	statusDraft := int8(jxc.SaleStatusDraft)
	list, total, err := ss.Search(ctx, jxcReq.DocumentSearch{
		PageInfo: common.PageInfo{Page: 1, PageSize: 20}, Status: &statusDraft,
	})
	if err != nil || total != 1 || list[0].ID != draft.ID {
		t.Fatalf("按待出库筛选失败: err=%v total=%d list=%+v", err, total, list)
	}
	list, total, err = ss.Search(ctx, jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20, Keyword: shipped.OrderNo}})
	if err != nil || total != 1 || list[0].ID != shipped.ID {
		t.Errorf("按销售单号筛选失败: err=%v total=%d list=%+v", err, total, list)
	}
}

func TestDocumentSearch_ReturnsAndReversal(t *testing.T) {
	db := newLedgerTestDB(t)
	wh, sku := seedSku(t, db)
	ps := NewPurchaseService()
	ss := NewSaleService()
	prs := NewPurchaseReturnService()
	rs := NewReturnService()
	rvs := NewReversalService()
	ctx := context.Background()

	// 销售退货：草稿可按原销售单号/退货单号筛选
	seedStock(t, db, wh, sku, 5, 10)
	so, err := ss.Create(ctx, &jxc.SaleOrder{WarehouseID: wh, Items: []jxc.SaleItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("开销售单失败: %v", err)
	}
	if err := ss.Ship(ctx, so.ID, "t", time.Now()); err != nil {
		t.Fatalf("出库失败: %v", err)
	}
	ret, err := rs.Create(ctx, &jxc.SaleReturn{SaleID: so.ID, Items: []jxc.SaleReturnItem{{SkuID: sku, Qty: 1}}})
	if err != nil {
		t.Fatalf("建退货单失败: %v", err)
	}
	list, total, err := rs.Search(ctx, jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20, Keyword: so.OrderNo}})
	if err != nil || total != 1 || list[0].ID != ret.ID {
		t.Fatalf("按原销售单号筛选退货失败: err=%v total=%d list=%+v", err, total, list)
	}

	// 采购退货：显示字段 + 关键词
	supplier := jxc.Partner{Kind: jxc.PartnerSupplier, Code: "SUP-2", Name: "辅料商", Status: 1}
	db.Create(&supplier)
	po, err := ps.Create(ctx, &jxc.PurchaseOrder{WarehouseID: wh, SupplierID: &supplier.ID,
		Items: []jxc.PurchaseItem{{SkuID: sku, Qty: 10, UnitCost: 5}}})
	if err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}
	_ = ps.Approve(ctx, po.ID, "t")
	if err := ps.Receive(ctx, po.ID, "t", time.Now()); err != nil {
		t.Fatalf("采购入库失败: %v", err)
	}
	pr, err := prs.Create(ctx, &jxc.PurchaseReturn{PurchaseID: po.ID,
		Items: []jxc.PurchaseReturnItem{{SkuID: sku, Qty: 2}}})
	if err != nil {
		t.Fatalf("建采购退货单失败: %v", err)
	}
	list2, total, err := prs.Search(ctx, jxcReq.DocumentSearch{PageInfo: common.PageInfo{Page: 1, PageSize: 20, Keyword: po.OrderNo}})
	if err != nil || total != 1 || list2[0].ID != pr.ID {
		t.Fatalf("按原采购单号筛选采购退货失败: err=%v total=%d list=%+v", err, total, list2)
	}
	if list2[0].SupplierName != "辅料商" || list2[0].WarehouseName != "主仓" || list2[0].ItemCount != 1 {
		t.Errorf("采购退货列表应带出供应商/仓库/明细数，实际 %+v", list2[0])
	}

	// 红冲：来源类型 + 单号筛选
	if _, err := rvs.Reverse(ctx, jxc.ReversalSourcePurchase, po.ID, "录错", "t"); err != nil {
		t.Fatalf("采购红冲失败: %v", err)
	}
	list3, total, err := rvs.Search(ctx, jxcReq.DocumentSearch{
		PageInfo: common.PageInfo{Page: 1, PageSize: 20}, SourceType: jxc.ReversalSourcePurchase,
	})
	if err != nil || total != 1 || list3[0].SourceID != po.ID {
		t.Fatalf("按来源类型筛选红冲失败: err=%v total=%d list=%+v", err, total, list3)
	}
	list3, total, err = rvs.Search(ctx, jxcReq.DocumentSearch{
		PageInfo: common.PageInfo{Page: 1, PageSize: 20}, WarehouseID: wh,
	})
	if err != nil || total != 1 || len(list3) != 1 {
		t.Errorf("按来源单仓库筛选红冲失败: err=%v total=%d", err, total)
	}
}
