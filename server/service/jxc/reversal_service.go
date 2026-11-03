package jxc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/internal/money"
	common "github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
	jxcReq "github.com/flipped-aurora/gin-vue-admin/server/model/jxc/request"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReversalService 红冲：整单撤销一张已过账的单据。
//
// 为什么必须有它：账本行的不可变性是这套设计的地基，但"不可变"如果没有出口，
// 就等于"录错了没法救"。红冲就是这个出口——不改历史，只追加一条反向事实。
//
// 两条硬约束：
//  1. 已红冲的单不能再红冲（唯一性靠 stock_reversal 的业务校验 + 账本行幂等键）；
//  2. 采购红冲后不能把库存冲成负数（货都卖了还整单红冲，账就假了）。
type ReversalService struct {
	ledger *LedgerService
}

func NewReversalService() *ReversalService {
	return &ReversalService{ledger: new(LedgerService)}
}

// Reverse 对一张采购单/销售单/采购退货单做整单红冲，记账时间为当前时间。
func (s *ReversalService) Reverse(ctx context.Context, sourceType string, sourceID uint, reason, operator string) (*jxc.StockReversal, error) {
	return s.ReverseAt(ctx, sourceType, sourceID, reason, operator, time.Now())
}

// ReverseAt 与 Reverse 相同，但允许指定红冲记账时间；用于测试和补录场景。
func (s *ReversalService) ReverseAt(ctx context.Context, sourceType string, sourceID uint, reason, operator string, postedAt time.Time) (*jxc.StockReversal, error) {
	if postedAt.IsZero() {
		postedAt = time.Now()
	}
	return s.reverseAt(ctx, sourceType, sourceID, reason, operator, postedAt)
}

func (s *ReversalService) reverseAt(ctx context.Context, sourceType string, sourceID uint, reason, operator string, postedAt time.Time) (*jxc.StockReversal, error) {
	if sourceID == 0 {
		return nil, errors.New("请选择要红冲的单据")
	}
	if sourceType != jxc.ReversalSourcePurchase && sourceType != jxc.ReversalSourceSale &&
		sourceType != jxc.ReversalSourcePurchaseReturn {
		return nil, errors.New("红冲来源只能是采购单、销售单或采购退货单")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("请填写红冲原因（这是日后解释账目变化的唯一线索）")
	}

	var rev *jxc.StockReversal
	err := global.GVA_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 校验原单据状态，并锁住它
		var (
			docNo   string
			moveSrc string
			docID   uint
		)
		switch sourceType {
		case jxc.ReversalSourcePurchase:
			var po jxc.PurchaseOrder
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&po, sourceID).Error; err != nil {
				return errors.New("采购单不存在")
			}
			if po.Status != jxc.PurchaseStatusReceived {
				return errors.New("只有已入库的采购单可以红冲")
			}
			docNo, moveSrc, docID = po.OrderNo, jxc.MoveSourcePurchaseIn, po.ID
		case jxc.ReversalSourceSale:
			var so jxc.SaleOrder
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&so, sourceID).Error; err != nil {
				return errors.New("销售单不存在")
			}
			if so.Status != jxc.SaleStatusShipped {
				return errors.New("只有已出库的销售单可以红冲")
			}
			// 已有退货的销售单不能整单红冲：退货已经回补过库存，再红冲会重复回补。
			// 草稿退货也一并挡住——否则红冲后这张草稿永远无法入库，只能取消，属于埋雷。
			var retCnt int64
			if err := tx.Model(&jxc.SaleReturn{}).
				Where("sale_id = ? AND status IN ?", so.ID,
					[]int8{jxc.ReturnStatusDraft, jxc.ReturnStatusPosted}).
				Count(&retCnt).Error; err != nil {
				return err
			}
			if retCnt > 0 {
				return errors.New("该销售单已存在退货单（草稿或已入库），不能整单红冲；请先取消退货单，或改用退货处理")
			}
			docNo, moveSrc, docID = so.OrderNo, jxc.MoveSourceSaleOut, so.ID
		case jxc.ReversalSourcePurchaseReturn:
			var pr jxc.PurchaseReturn
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pr, sourceID).Error; err != nil {
				return errors.New("采购退货单不存在")
			}
			if pr.Status != jxc.PurchaseReturnStatusPosted {
				return errors.New("只有已退货的采购退货单可以红冲")
			}
			if pr.ReversedAt != nil {
				return errors.New("该采购退货单已经红冲过了")
			}
			docNo, moveSrc, docID = pr.ReturnNo, jxc.MoveSourcePurchaseReturn, pr.ID
		}

		// 2) 不能重复红冲
		var exist int64
		if err := tx.Model(&jxc.StockReversal{}).
			Where("source_type = ? AND source_id = ?", sourceType, docID).Count(&exist).Error; err != nil {
			return err
		}
		if exist > 0 {
			return errors.New("该单据已经红冲过了")
		}

		// 3) 找出原始账本行
		var orig []jxc.StockMove
		if err := tx.Where("source_type = ? AND source_id = ?", moveSrc, docID).
			Order("id").Find(&orig).Error; err != nil {
			return err
		}
		if len(orig) == 0 {
			return errors.New("未找到该单据的账本记录，无法红冲")
		}

		// 4) 采购红冲会让库存减少：不能冲成负数
		if sourceType == jxc.ReversalSourcePurchase {
			need := map[uint]int{}
			whID := uint(0)
			for _, m := range orig {
				// 采购入库的账本行 qty 为正，红冲要扣掉的就是这个正数
				need[m.SkuID] += m.Qty
				whID = m.WarehouseID
			}
			for skuID, q := range need {
				var b jxc.StockBalance
				if err := tx.Where("warehouse_id = ? AND sku_id = ?", whID, skuID).First(&b).Error; err != nil {
					return fmt.Errorf("查不到结存，无法红冲：SKU %d", skuID)
				}
				reserved, err := s.ledger.reservedQty(tx, whID, skuID)
				if err != nil {
					return err
				}
				if b.Qty-reserved < q {
					return fmt.Errorf("该批货已卖出（SKU %d 可售 %d，需冲回 %d），不能整单红冲；请改用盘亏或退货处理",
						skuID, b.Qty-reserved, q)
				}
			}
		}

		// 5) 建红冲单（先拿到 ID 作为账本行的来源）
		var qtySum int
		var amtSum money.Cents
		for _, m := range orig {
			qtySum += absInt(m.Qty)
			amtSum = amtSum.Add(money.FromYuan(m.Amount).Abs())
		}
		no, err := nextOrderNo(tx, "RC")
		if err != nil {
			return err
		}
		rev = &jxc.StockReversal{
			ReversalNo: no, SourceType: sourceType, SourceID: docID, SourceNo: docNo,
			Reason: reason, Amount: amtSum.Yuan(), Qty: qtySum,
			Status: jxc.ReversalPosted, PostedAt: postedAt, Operator: operator,
		}
		if err := tx.Create(rev).Error; err != nil {
			return err
		}

		// 6) 反向记账：数量取反，成本沿用原值（这样金额精确抵消，结存回到红冲前）
		moves := make([]MoveInput, 0, len(orig))
		for _, m := range orig {
			origID := m.ID
			moves = append(moves, MoveInput{
				WarehouseID: m.WarehouseID, SkuID: m.SkuID,
				Qty: -m.Qty, UnitCost: m.UnitCost,
				SourceType: jxc.MoveSourceReversal, SourceID: rev.ID, SourceLineID: origID,
				OccurredAt: rev.PostedAt, Operator: operator, ReversalOf: &origID,
			})
		}
		if err := s.ledger.Post(ctx, tx, moves); err != nil {
			return err
		}

		// 7) 标记原单据（保留状态，只加时间戳，避免状态码语义膨胀）
		now := time.Now()
		table := "purchase_order"
		switch sourceType {
		case jxc.ReversalSourceSale:
			table = "sale_order"
		case jxc.ReversalSourcePurchaseReturn:
			table = "purchase_return"
		}
		return tx.Table(table).Where("id = ?", docID).Update("reversed_at", now).Error
	})
	if err != nil {
		return nil, err
	}
	return rev, nil
}

// Page 红冲记录（兼容旧调用；新代码用 Search）
func (s *ReversalService) Page(ctx context.Context, page, size int) ([]jxc.StockReversal, int64, error) {
	return s.Search(ctx, jxcReq.DocumentSearch{
		PageInfo: common.PageInfo{Page: page, PageSize: size},
	})
}

// Search 红冲记录的服务端筛选。
// 红冲单本身没有仓库/往来单位字段，因此通过来源单 join 支持这两个筛选；
// 关键词按红冲单号、原单号、原因搜索（来源类型用 sourceType 精确筛选）。
func (s *ReversalService) Search(ctx context.Context, info jxcReq.DocumentSearch) ([]jxc.StockReversal, int64, error) {
	info.Normalize()
	db := global.GVA_DB.WithContext(ctx).Model(&jxc.StockReversal{}).
		Joins("LEFT JOIN sale_order so ON stock_reversal.source_type = 'sale' AND so.id = stock_reversal.source_id").
		Joins("LEFT JOIN purchase_order po ON stock_reversal.source_type = 'purchase' AND po.id = stock_reversal.source_id").
		Joins("LEFT JOIN purchase_return pr ON stock_reversal.source_type = 'purchase_return' AND pr.id = stock_reversal.source_id")

	if info.Status != nil && *info.Status != jxc.ReversalPosted {
		db = db.Where("1 = 0")
	}
	if info.SourceType != "" {
		if info.SourceType != jxc.ReversalSourcePurchase && info.SourceType != jxc.ReversalSourceSale &&
			info.SourceType != jxc.ReversalSourcePurchaseReturn {
			return nil, 0, errors.New("sourceType 只能是 purchase、sale 或 purchase_return")
		}
		db = db.Where("stock_reversal.source_type = ?", info.SourceType)
	}
	if info.WarehouseID > 0 {
		db = db.Where("((stock_reversal.source_type = 'sale' AND so.warehouse_id = ?) OR (stock_reversal.source_type = 'purchase' AND po.warehouse_id = ?) OR (stock_reversal.source_type = 'purchase_return' AND pr.warehouse_id = ?))",
			info.WarehouseID, info.WarehouseID, info.WarehouseID)
	}
	if info.PartnerID > 0 {
		db = db.Where("((stock_reversal.source_type = 'sale' AND so.customer_id = ?) OR (stock_reversal.source_type = 'purchase' AND po.supplier_id = ?) OR (stock_reversal.source_type = 'purchase_return' AND pr.supplier_id = ?))",
			info.PartnerID, info.PartnerID, info.PartnerID)
	}
	if kw := keywordPattern(info.Keyword); kw != "" {
		db = db.Where("(stock_reversal.reversal_no LIKE ? OR stock_reversal.source_no LIKE ? OR stock_reversal.reason LIKE ?)",
			kw, kw, kw)
	}
	db, err := applyDocumentDateRange(db, info, "stock_reversal.created_at", "stock_reversal.posted_at")
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit, offset := info.LimitOffset()
	var list []jxc.StockReversal
	err = db.Order("stock_reversal.id desc").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
