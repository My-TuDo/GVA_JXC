package jxc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
	"gorm.io/gorm"
)

// SaleService 销售中心服务
type SaleService struct{}

// ========== 辅助 ==========

// genSaleOrderNo 生成销售单号 SO-YYYYMMDD-XXX
// 用 Unscoped 包含软删除行（避免删除后重建撞唯一索引）
func (s *SaleService) genSaleOrderNo(db *gorm.DB) (string, error) {
	date := time.Now().Format("20060102")
	like := "SO-" + date + "%"
	var maxNo string
	if err := db.Unscoped().Model(&jxc.SaleOrder{}).Where("order_no LIKE ?", like).
		Order("order_no DESC").Limit(1).Pluck("order_no", &maxNo).Error; err != nil {
		return "", err
	}
	if maxNo == "" {
		return "SO-" + date + "-001", nil
	}
	seq, err := strconv.Atoi(maxNo[len(maxNo)-3:])
	if err != nil {
		return "", fmt.Errorf("解析单号序号失败: %s", maxNo)
	}
	return fmt.Sprintf("SO-%s-%03d", date, seq+1), nil
}

// isDuplicateKey 判断是否为唯一索引冲突（MySQL 1062 / SQLite UNIQUE constraint）
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") || strings.Contains(msg, "UNIQUE constraint failed")
}

// lockStock 锁定库存（校验可售库存充足）
func (s *SaleService) lockStock(tx *gorm.DB, warehouseID, skuID uint, qty int) error {
	var stock jxc.Stock
	if err := tx.Where("warehouse_id = ? AND sku_id = ?", warehouseID, skuID).First(&stock).Error; err != nil {
		return errors.New("该 SKU 在该仓库无库存记录，请先采购入库")
	}
	available := stock.Quantity - stock.LockQuantity
	if available < qty {
		return fmt.Errorf("库存不足：可售库存 %d，需 %d", available, qty)
	}
	return tx.Model(&stock).Update("lock_quantity", stock.LockQuantity+qty).Error
}

// unlockStock 释放锁定库存
func (s *SaleService) unlockStock(tx *gorm.DB, warehouseID, skuID uint, qty int) error {
	var stock jxc.Stock
	if err := tx.Where("warehouse_id = ? AND sku_id = ?", warehouseID, skuID).First(&stock).Error; err != nil {
		return err
	}
	return tx.Model(&stock).Update("lock_quantity", stock.LockQuantity-qty).Error
}

// fillItemSnapshot 填充明细快照并汇总金额
// 单价以 SKU 销售价为准；金额符号：正常销售正 / 退货负 / 换货按 direction（1换出正 2换入负）
func (s *SaleService) fillItemSnapshot(db *gorm.DB, orderType int8, items []jxc.SaleItem) (float64, error) {
	// 换货单必须同时包含换出（direction=1）与换入（direction=2）明细
	if orderType == jxc.SaleTypeExchange {
		hasOut, hasIn := false, false
		for _, it := range items {
			if it.Direction == 1 {
				hasOut = true
			}
			if it.Direction == 2 {
				hasIn = true
			}
		}
		if !hasOut || !hasIn {
			return 0, errors.New("换货单必须同时包含换出和换入明细")
		}
	}
	var total float64
	for i := range items {
		it := &items[i]
		if it.Qty <= 0 {
			return 0, errors.New("销售数量必须大于 0")
		}
		var sku jxc.GoodsSku
		if err := db.First(&sku, it.SkuID).Error; err != nil {
			return 0, errors.New("SKU 不存在")
		}
		// 单价固定取 SKU 销售价，前端传入的 price 一律忽略（角色改价权限后续版本开放）
		it.Price = sku.SalePrice
		it.Amount = float64(it.Qty) * it.Price
		switch orderType {
		case jxc.SaleTypeReturn:
			it.Amount = -it.Amount // 退货：收入减少
		case jxc.SaleTypeExchange:
			switch it.Direction {
			case 1: // 换出：正向流水（销售延续）
			case 2: // 换入：负向流水（相当于退回）
				it.Amount = -it.Amount
			default:
				return 0, errors.New("换货明细必须指定方向（1换出/2换入）")
			}
		}
		total += it.Amount
		it.SkuCode = sku.SkuCode
		it.Color = sku.Color
		it.Size = sku.Size
		var goods jxc.Goods
		db.First(&goods, sku.GoodsID)
		it.GoodsName = goods.Name
	}
	return total, nil
}

// checkOriginalOrder 校验关联原单（退货/换货可关联正常销售已出库或换货已完成；换出的商品可继续退/换）
func (s *SaleService) checkOriginalOrder(db *gorm.DB, orderType int8, originalID *uint) error {
	if originalID == nil || *originalID == 0 {
		return errors.New("请选择关联原单")
	}
	var original jxc.SaleOrder
	if err := db.First(&original, *originalID).Error; err != nil {
		return errors.New("关联原单不存在")
	}
	if original.Status != jxc.SaleStatusShipped {
		return errors.New("仅已出库的销售单可作为原单")
	}
	switch orderType {
	case jxc.SaleTypeReturn, jxc.SaleTypeExchange:
		if original.OrderType != jxc.SaleTypeNormal && original.OrderType != jxc.SaleTypeExchange {
			return errors.New("退货/换货单只能关联正常销售或换货单")
		}
	}
	return nil
}

// calcInboundRemaining 计算原单剩余可退换件数（出库总件数 − 已确认占用总件数）
// 原单出库：正常销售 = 全部明细；换货单 = 换出明细（direction=1）
// 已占用：该原单下已确认退/换单的入库明细（退货 direction=0、换入 direction=2）
func (s *SaleService) calcInboundRemaining(db *gorm.DB, originalID uint, excludeID uint) (outQty, usedQty int, err error) {
	var outRow struct{ Qty int }
	if err = db.Table("sale_item si").
		Select("COALESCE(SUM(si.qty), 0) AS qty").
		Joins("JOIN sale_order o ON o.id = si.sale_id").
		Where("si.sale_id = ? AND (o.order_type = ? OR si.direction = ?)", originalID, jxc.SaleTypeNormal, 1).
		Scan(&outRow).Error; err != nil {
		return 0, 0, err
	}
	var usedRow struct{ Qty int }
	if err = db.Table("sale_item si").
		Select("COALESCE(SUM(si.qty), 0) AS qty").
		Joins("JOIN sale_order o ON o.id = si.sale_id").
		Where("o.original_order_id = ? AND o.status = ? AND o.id <> ? AND si.direction IN (0, 2)", originalID, jxc.SaleStatusShipped, excludeID).
		Scan(&usedRow).Error; err != nil {
		return 0, 0, err
	}
	return outRow.Qty, usedRow.Qty, nil
}

// checkInboundLimit 校验入库数量上限（按单据件数总量）：
// 退货/换货单的入库总件数不得超过原单出库总件数减去已被其他已确认单据占用的件数
// 允许换不同商品（额度按件数，不限定商品），防止无中生有
func (s *SaleService) checkInboundLimit(db *gorm.DB, orderType int8, originalID uint, items []jxc.SaleItem, excludeID uint) error {
	if orderType != jxc.SaleTypeReturn && orderType != jxc.SaleTypeExchange {
		return nil
	}
	// 当前单据入库总件数
	inQty := 0
	for _, it := range items {
		isIn := orderType == jxc.SaleTypeReturn || (orderType == jxc.SaleTypeExchange && it.Direction == 2)
		if isIn {
			inQty += it.Qty
		}
	}
	if inQty == 0 {
		return nil
	}
	outQty, usedQty, err := s.calcInboundRemaining(db, originalID, excludeID)
	if err != nil {
		return err
	}
	limit := outQty - usedQty
	if inQty > limit {
		return fmt.Errorf("入库件数超出原单剩余可退换数量：原单出库 %d 件，已占用 %d 件，最多还可入库 %d 件", outQty, usedQty, limit)
	}
	return nil
}

// checkReturnSkus 校验退货明细必须是原单出库过的 SKU（业界退货限定原单商品；换货才允许换不同商品）
func (s *SaleService) checkReturnSkus(db *gorm.DB, orderType int8, originalID uint, items []jxc.SaleItem) error {
	if orderType != jxc.SaleTypeReturn {
		return nil
	}
	var skuIDs []uint
	db.Table("sale_item si").
		Select("DISTINCT si.sku_id").
		Joins("JOIN sale_order o ON o.id = si.sale_id").
		Where("si.sale_id = ? AND (o.order_type = ? OR si.direction = ?)", originalID, jxc.SaleTypeNormal, 1).
		Scan(&skuIDs)
	allowed := map[uint]bool{}
	for _, id := range skuIDs {
		allowed[id] = true
	}
	for _, it := range items {
		if !allowed[it.SkuID] {
			return fmt.Errorf("退货商品（SKU %d）不在原单出库商品中", it.SkuID)
		}
	}
	return nil
}

// GetRemaining 查询原单剩余可退换件数（供前端数量上限钳制）
func (s *SaleService) GetRemaining(ctx context.Context, originalID uint) (jxc.SaleRemaining, error) {
	db := global.GVA_DB.WithContext(ctx)
	var original jxc.SaleOrder
	if err := db.First(&original, originalID).Error; err != nil {
		return jxc.SaleRemaining{}, errors.New("关联原单不存在")
	}
	outQty, usedQty, err := s.calcInboundRemaining(db, originalID, 0)
	if err != nil {
		return jxc.SaleRemaining{}, err
	}
	return jxc.SaleRemaining{
		OrderNo:   original.OrderNo,
		OutQty:    outQty,
		UsedQty:   usedQty,
		Remaining: outQty - usedQty,
	}, nil
}

// ========== 销售单 CRUD ==========

// CreateSaleOrder 创建销售单（待出库, 正常销售锁定库存）
// 单号并发/删除重建冲突时自动重试重新生成
func (s *SaleService) CreateSaleOrder(ctx context.Context, order *jxc.SaleOrder) error {
	if len(order.Items) == 0 {
		return errors.New("销售单至少需要一条明细")
	}
	order.Status = jxc.SaleStatusPending
	if order.OrderType == 0 {
		order.OrderType = jxc.SaleTypeNormal
	}
	itemsCopy := order.Items
	for attempt := 0; attempt < 3; attempt++ {
		order.Items = itemsCopy
		order.OrderNo = ""
		order.ID = 0
		err := s.createSaleOrderTx(ctx, order)
		if err == nil {
			return nil
		}
		if !isDuplicateKey(err) {
			return err
		}
		// 单号冲突：重试（createSaleOrderTx 会重新生成单号）
	}
	return errors.New("单号生成冲突，请重试")
}

// createSaleOrderTx 创建销售单的事务体（单号生成 + 校验 + 落库）
func (s *SaleService) createSaleOrderTx(ctx context.Context, order *jxc.SaleOrder) error {
	db := global.GVA_DB.WithContext(ctx)
	orderNo, err := s.genSaleOrderNo(db)
	if err != nil {
		return err
	}
	order.OrderNo = orderNo
	// 退货/换货必须关联合法原单
	if order.OrderType != jxc.SaleTypeNormal {
		if err := s.checkOriginalOrder(db, order.OrderType, order.OriginalOrderID); err != nil {
			return err
		}
		if err := s.checkReturnSkus(db, order.OrderType, *order.OriginalOrderID, order.Items); err != nil {
			return err
		}
		if err := s.checkInboundLimit(db, order.OrderType, *order.OriginalOrderID, order.Items, 0); err != nil {
			return err
		}
	}
	total, err := s.fillItemSnapshot(db, order.OrderType, order.Items)
	if err != nil {
		return err
	}
	order.TotalAmount = total

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		// 正常销售：建单即锁定库存
		if order.OrderType == jxc.SaleTypeNormal {
			for i := range order.Items {
				if err := s.lockStock(tx, order.WarehouseID, order.Items[i].SkuID, order.Items[i].Qty); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetSalePage 分页查询销售单（支持 created_at 日期范围过滤，供看板历史统计下钻）
func (s *SaleService) GetSalePage(ctx context.Context, info jxc.SaleQuery) (list []jxc.SaleOrder, total int64, err error) {
	db := global.GVA_DB.WithContext(ctx).Model(&jxc.SaleOrder{})
	if info.Keyword != "" {
		db = db.Where("order_no LIKE ?", "%"+info.Keyword+"%")
	}
	if info.From != "" {
		db = db.Where("created_at >= ?", info.From)
	}
	if info.To != "" {
		// 含当天：右区间开区间
		db = db.Where("created_at < ?", nextDay(info.To))
	}
	if err = db.Count(&total).Error; err != nil || total == 0 {
		return
	}
	limit, offset := info.LimitOffset()
	err = db.Preload("Customer").Preload("Warehouse").
		Limit(limit).Offset(offset).Order("id desc").Find(&list).Error
	return
}

// GetSaleDetail 查询销售单详情（含明细）
func (s *SaleService) GetSaleDetail(ctx context.Context, id uint) (order jxc.SaleOrder, err error) {
	err = global.GVA_DB.WithContext(ctx).
		Preload("Customer").Preload("Warehouse").Preload("Items").Preload("Original").
		First(&order, id).Error
	return
}

// UpdateSaleOrder 更新销售单（仅待出库, 明细重建并重新锁定库存）
func (s *SaleService) UpdateSaleOrder(ctx context.Context, order *jxc.SaleOrder) error {
	if len(order.Items) == 0 {
		return errors.New("销售单至少需要一条明细")
	}
	db := global.GVA_DB.WithContext(ctx)
	var old jxc.SaleOrder
	if err := db.First(&old, order.ID).Error; err != nil {
		return err
	}
	if old.Status != jxc.SaleStatusPending {
		return errors.New("仅待出库状态的销售单可以修改")
	}
	// 退货/换货必须关联合法原单
	if order.OrderType != jxc.SaleTypeNormal {
		if err := s.checkOriginalOrder(db, order.OrderType, order.OriginalOrderID); err != nil {
			return err
		}
		if err := s.checkReturnSkus(db, order.OrderType, *order.OriginalOrderID, order.Items); err != nil {
			return err
		}
		if err := s.checkInboundLimit(db, order.OrderType, *order.OriginalOrderID, order.Items, order.ID); err != nil {
			return err
		}
	}
	total, err := s.fillItemSnapshot(db, order.OrderType, order.Items)
	if err != nil {
		return err
	}
	order.TotalAmount = total
	order.OrderNo = old.OrderNo
	order.Status = old.Status

	return db.Transaction(func(tx *gorm.DB) error {
		// 解锁旧明细
		if old.OrderType == jxc.SaleTypeNormal {
			var oldItems []jxc.SaleItem
			if err := tx.Where("sale_id = ?", order.ID).Find(&oldItems).Error; err != nil {
				return err
			}
			for i := range oldItems {
				if err := s.unlockStock(tx, old.WarehouseID, oldItems[i].SkuID, oldItems[i].Qty); err != nil {
					return err
				}
			}
		}
		if err := tx.Omit("created_at").Save(order).Error; err != nil {
			return err
		}
		if err := tx.Where("sale_id = ?", order.ID).Delete(&jxc.SaleItem{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&order.Items).Error; err != nil {
			return err
		}
		// 锁定新明细
		if order.OrderType == jxc.SaleTypeNormal {
			for i := range order.Items {
				if err := s.lockStock(tx, order.WarehouseID, order.Items[i].SkuID, order.Items[i].Qty); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteSaleOrder 删除销售单（仅待出库, 释放锁定）
func (s *SaleService) DeleteSaleOrder(ctx context.Context, id uint) error {
	db := global.GVA_DB.WithContext(ctx)
	var old jxc.SaleOrder
	if err := db.First(&old, id).Error; err != nil {
		return err
	}
	if old.Status != jxc.SaleStatusPending {
		return errors.New("仅待出库状态的销售单可以删除")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if old.OrderType == jxc.SaleTypeNormal {
			var items []jxc.SaleItem
			if err := tx.Where("sale_id = ?", id).Find(&items).Error; err != nil {
				return err
			}
			for i := range items {
				if err := s.unlockStock(tx, old.WarehouseID, items[i].SkuID, items[i].Qty); err != nil {
					return err
				}
			}
		}
		if err := tx.Delete(&jxc.SaleOrder{}, id).Error; err != nil {
			return err
		}
		return tx.Where("sale_id = ?", id).Delete(&jxc.SaleItem{}).Error
	})
}

// ========== 状态流转 ==========

// CancelSaleOrder 取消销售单（待出库 → 已取消, 释放锁定）
func (s *SaleService) CancelSaleOrder(ctx context.Context, id uint) error {
	return global.GVA_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order jxc.SaleOrder
		if err := tx.First(&order, id).Error; err != nil {
			return err
		}
		if order.Status != jxc.SaleStatusPending {
			return errors.New("仅待出库状态的销售单可以取消")
		}
		if order.OrderType == jxc.SaleTypeNormal {
			var items []jxc.SaleItem
			if err := tx.Where("sale_id = ?", id).Find(&items).Error; err != nil {
				return err
			}
			for i := range items {
				if err := s.unlockStock(tx, order.WarehouseID, items[i].SkuID, items[i].Qty); err != nil {
					return err
				}
			}
		}
		return tx.Model(&jxc.SaleOrder{}).Where("id = ?", id).
			Update("status", jxc.SaleStatusCanceled).Error
	})
}

// ConfirmOut 确认出库（待出库 → 已出库, 扣减库存+流水）
func (s *SaleService) ConfirmOut(ctx context.Context, id uint, operator string) error {
	return global.GVA_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order jxc.SaleOrder
		if err := tx.First(&order, id).Error; err != nil {
			return err
		}
		if order.Status != jxc.SaleStatusPending {
			return errors.New("仅待出库状态的销售单可以出库")
		}
		var items []jxc.SaleItem
		if err := tx.Where("sale_id = ?", id).Find(&items).Error; err != nil {
			return err
		}
		for i := range items {
			it := &items[i]
			var stock jxc.Stock
			if err := tx.Where("warehouse_id = ? AND sku_id = ?", order.WarehouseID, it.SkuID).First(&stock).Error; err != nil {
				return errors.New("库存记录不存在")
			}
			if stock.Quantity < it.Qty {
				return fmt.Errorf("库存不足：SKU %s 现有 %d，需 %d", it.SkuCode, stock.Quantity, it.Qty)
			}
			// 更新前捕获数量（GORM Updates 会回填 struct）
			beforeQty := stock.Quantity
			// 扣减数量与锁定
			if err := tx.Model(&stock).
				Updates(map[string]interface{}{"quantity": stock.Quantity - it.Qty, "lock_quantity": stock.LockQuantity - it.Qty}).Error; err != nil {
				return err
			}
			// 流水
			log := jxc.StockLog{
				WarehouseID: order.WarehouseID, SkuID: it.SkuID,
				BusinessType: "sale_out", BusinessNo: order.OrderNo,
				BeforeQty: beforeQty, ChangeQty: -it.Qty, AfterQty: beforeQty - it.Qty,
				Operator: operator,
			}
			if err := tx.Create(&log).Error; err != nil {
				return err
			}
		}
		return tx.Model(&jxc.SaleOrder{}).Where("id = ?", id).
			Update("status", jxc.SaleStatusShipped).Error
	})
}

// ConfirmReturn 确认退货入库（退货单 → 已出库, 恢复库存+流水）
func (s *SaleService) ConfirmReturn(ctx context.Context, id uint, operator string) error {
	return global.GVA_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.confirmReturnTx(tx, id, operator)
	})
}

// confirmReturnTx 退货入库事务体（供 ConfirmReturn 与收银退款/换货复用，须在事务内调用）
func (s *SaleService) confirmReturnTx(tx *gorm.DB, id uint, operator string) error {
	var order jxc.SaleOrder
	if err := tx.First(&order, id).Error; err != nil {
		return err
	}
	if order.OrderType != jxc.SaleTypeReturn {
		return errors.New("仅退货单可以执行退货入库")
	}
	if order.Status != jxc.SaleStatusPending {
		return errors.New("仅待出库状态的退货单可以入库")
	}
	var items []jxc.SaleItem
	if err := tx.Where("sale_id = ?", id).Find(&items).Error; err != nil {
		return err
	}
	// 确认时硬校验入库上限：其他已确认占用 + 本次入库 ≤ 原单出库（禁止虚增）
	if order.OriginalOrderID != nil {
		if err := s.checkInboundLimit(tx, order.OrderType, *order.OriginalOrderID, items, 0); err != nil {
			return err
		}
	}
	for i := range items {
		it := &items[i]
		var stock jxc.Stock
		err := tx.Where("warehouse_id = ? AND sku_id = ?", order.WarehouseID, it.SkuID).First(&stock).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		beforeQty := stock.Quantity
		if err == gorm.ErrRecordNotFound {
			stock = jxc.Stock{WarehouseID: order.WarehouseID, SkuID: it.SkuID, Quantity: it.Qty}
			if err := tx.Create(&stock).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&stock).Update("quantity", stock.Quantity+it.Qty).Error; err != nil {
				return err
			}
		}
		log := jxc.StockLog{
			WarehouseID: order.WarehouseID, SkuID: it.SkuID,
			BusinessType: "sale_return", BusinessNo: order.OrderNo,
			BeforeQty: beforeQty, ChangeQty: it.Qty, AfterQty: beforeQty + it.Qty,
			Operator: operator,
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
	}
	return tx.Model(&jxc.SaleOrder{}).Where("id = ?", id).
		Update("status", jxc.SaleStatusShipped).Error
}

// ConfirmExchange 确认换货（换货单 → 已完成: 负明细换出扣库存, 正明细换入加库存, 联合确认）
func (s *SaleService) ConfirmExchange(ctx context.Context, id uint, operator string) error {
	return global.GVA_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order jxc.SaleOrder
		if err := tx.First(&order, id).Error; err != nil {
			return err
		}
		if order.OrderType != jxc.SaleTypeExchange {
			return errors.New("仅换货单可以执行换货确认")
		}
		if order.Status != jxc.SaleStatusPending {
			return errors.New("仅待确认状态的换货单可以执行换货确认")
		}
		var items []jxc.SaleItem
		if err := tx.Where("sale_id = ?", id).Find(&items).Error; err != nil {
			return err
		}
		// 确认时硬校验入库上限：其他已确认占用 + 本次换入 ≤ 原单出库（禁止虚增）
		if order.OriginalOrderID != nil {
			if err := s.checkInboundLimit(tx, order.OrderType, *order.OriginalOrderID, items, 0); err != nil {
				return err
			}
		}
		for i := range items {
			it := &items[i]
			var stock jxc.Stock
			err := tx.Where("warehouse_id = ? AND sku_id = ?", order.WarehouseID, it.SkuID).First(&stock).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// 更新前捕获数量（GORM Update 会回填 struct）
			beforeQty := stock.Quantity
			switch it.Direction {
			case 1:
				// 换出明细：扣减库存（出为正）
				if err == gorm.ErrRecordNotFound || stock.Quantity < it.Qty {
					return fmt.Errorf("库存不足：SKU %s 需换出 %d", it.SkuCode, it.Qty)
				}
				if err := tx.Model(&stock).Update("quantity", stock.Quantity-it.Qty).Error; err != nil {
					return err
				}
			case 2:
				// 换入明细：增加库存（入为负）
				if err == gorm.ErrRecordNotFound {
					beforeQty = 0
					stock = jxc.Stock{WarehouseID: order.WarehouseID, SkuID: it.SkuID, Quantity: it.Qty}
					if err := tx.Create(&stock).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Model(&stock).Update("quantity", stock.Quantity+it.Qty).Error; err != nil {
						return err
					}
				}
			default:
				return errors.New("换货明细方向缺失")
			}
			// 流水（换出 ChangeQty 为负, 换入为正）
			businessType := "sale_exchange_out"
			changeQty := -it.Qty
			if it.Direction == 2 {
				businessType = "sale_exchange_in"
				changeQty = it.Qty
			}
			log := jxc.StockLog{
				WarehouseID: order.WarehouseID, SkuID: it.SkuID,
				BusinessType: businessType, BusinessNo: order.OrderNo,
				BeforeQty: beforeQty, ChangeQty: changeQty, AfterQty: beforeQty + changeQty,
				Operator: operator,
			}
			if err := tx.Create(&log).Error; err != nil {
				return err
			}
		}
		return tx.Model(&jxc.SaleOrder{}).Where("id = ?", id).
			Update("status", jxc.SaleStatusShipped).Error
	})
}
