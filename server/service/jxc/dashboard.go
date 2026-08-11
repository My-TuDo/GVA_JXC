package jxc

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
)

// DashboardService 仪表盘聚合
type DashboardService struct{}

// OverviewItem 概览（今日/本周/本月）
type OverviewItem struct {
	Label    string  `json:"label"`
	Sales    float64 `json:"sales"`    // 净销售额（含退换符号）
	Orders   int64   `json:"orders"`   // 订单数（已确认单）
	Profit   float64 `json:"profit"`   // 毛利 = Σ(amount) - Σ(qty*成本)
	ReturnAmt float64 `json:"returnAmt"` // 退货额（退货+换入的负数部分）
}

// TrendPoint 趋势点
type TrendPoint struct {
	Date   string  `json:"date"`
	Sales  float64 `json:"sales"`
	Orders int64   `json:"orders"`
}

// TopItem 热销
type TopItem struct {
	SkuCode  string  `json:"skuCode"`
	GoodsName string `json:"goodsName"`
	Qty      int     `json:"qty"`
	Amount   float64 `json:"amount"`
}

// AlertItem 库存预警
type AlertItem struct {
	SkuID     uint   `json:"skuId"`
	SkuCode   string `json:"skuCode"`
	GoodsName string `json:"goodsName"`
	SafeStock int    `json:"safeStock"`
	Available int    `json:"available"`
}

// CatItem 分类占比
type CatItem struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

// SalesHistoryItem 历史销售统计点（按日/按月）
type SalesHistoryItem struct {
	Date   string  `json:"date"`
	Sales  float64 `json:"sales"`
	Orders int64   `json:"orders"`
	Profit float64 `json:"profit"`
}

// Overview 今日/本周/本月概览
func (s *DashboardService) Overview(ctx context.Context) ([]OverviewItem, error) {
	db := global.GVA_DB.WithContext(ctx)
	now := time.Now()
	// 统一上界：明天（各周期均覆盖到今天为止的全部数据）
	to := nextDay(now.Format("2006-01-02"))
	periods := []struct {
		label string
		from  string
	}{
		{"今日", now.Format("2006-01-02")},
		{"本周", mondayOf(now).Format("2006-01-02")},
		{"本月", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")},
	}
	result := make([]OverviewItem, 0, 3)
	for _, p := range periods {
		var sales struct {
			Total  float64
			Orders int64
		}
		if err := db.Table("sale_order").
			Select("COALESCE(SUM(total_amount),0) AS total, COUNT(*) AS orders").
			Where("status = ? AND created_at >= ? AND created_at < ?", 2, p.from, to).
			Scan(&sales).Error; err != nil {
			return nil, err
		}
		var profit struct {
			Total     float64
			ReturnAmt float64
		}
		if err := db.Table("sale_item i").
			Select("COALESCE(SUM(i.amount - i.qty * COALESCE(sku.cost_price,0) * CASE WHEN i.amount >= 0 THEN 1 ELSE -1 END),0) AS total, "+
				"COALESCE(SUM(CASE WHEN i.amount < 0 THEN i.amount ELSE 0 END),0) AS return_amt").
			Joins("JOIN sale_order o ON o.id = i.sale_id").
			Joins("JOIN goods_sku sku ON sku.id = i.sku_id").
			Where("o.status = ? AND o.created_at >= ? AND o.created_at < ?", 2, p.from, to).
			Scan(&profit).Error; err != nil {
			return nil, err
		}
		result = append(result, OverviewItem{
			Label: p.label, Sales: sales.Total, Orders: sales.Orders,
			Profit: profit.Total, ReturnAmt: profit.ReturnAmt,
		})
	}
	return result, nil
}

// Trend 近 N 天销售趋势
func (s *DashboardService) Trend(ctx context.Context, days int) ([]TrendPoint, error) {
	if days <= 0 {
		days = 7
	}
	db := global.GVA_DB.WithContext(ctx)
	from := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var list []TrendPoint
	err := db.Table("sale_order").
		Select("SUBSTR(created_at, 1, 10) AS date, COALESCE(SUM(total_amount),0) AS sales, COUNT(*) AS orders").
		Where("status = ? AND created_at >= ?", 2, from).
		Group("SUBSTR(created_at, 1, 10)").Order("date").Scan(&list).Error
	// 防御：MySQL parseTime=True 时 DATE 列可能以 RFC3339 形式落到 string（如 2026-08-04T00:00:00+08:00），统一截断
	for i := range list {
		list[i].Date = trimDateT(list[i].Date)
	}
	return list, err
}

// trimDateT 截断日期字符串中的 T 之后部分（兼容 ISO 时间戳）
func trimDateT(d string) string {
	if idx := strings.IndexByte(d, 'T'); idx >= 0 {
		return d[:idx]
	}
	return d
}

// Top 热销商品（正常销售 + 换出，按销量）
func (s *DashboardService) Top(ctx context.Context, days, limit int) ([]TopItem, error) {
	if days <= 0 {
		days = 30
	}
	if limit <= 0 {
		limit = 10
	}
	db := global.GVA_DB.WithContext(ctx)
	from := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var list []TopItem
	err := db.Table("sale_item i").
		Select("sku.sku_code AS sku_code, COALESCE(g.name,'') AS goods_name, "+
			"SUM(i.qty) AS qty, COALESCE(SUM(i.amount),0) AS amount").
		Joins("JOIN sale_order o ON o.id = i.sale_id").
		Joins("JOIN goods_sku sku ON sku.id = i.sku_id").
		Joins("JOIN goods g ON g.id = sku.goods_id").
		Where("o.status = ? AND (o.order_type = ? OR i.direction = ?) AND o.created_at >= ?", 2, 1, 1, from).
		Group("sku.id").Order("qty DESC").Limit(limit).Scan(&list).Error
	return list, err
}

// StockAlert 库存预警（可售 ≤ 安全库存，safe_stock > 0）
func (s *DashboardService) StockAlert(ctx context.Context) ([]AlertItem, error) {
	db := global.GVA_DB.WithContext(ctx)
	var list []AlertItem
	err := db.Table("goods_sku sku").
		Select("sku.id AS sku_id, sku.sku_code, COALESCE(g.name,'') AS goods_name, "+
			"sku.safe_stock, COALESCE(SUM(st.quantity),0) - COALESCE(SUM(st.lock_quantity),0) AS available").
		Joins("JOIN goods g ON g.id = sku.goods_id").
		Joins("LEFT JOIN stock st ON st.sku_id = sku.id").
		Where("sku.safe_stock > 0 AND sku.deleted_at IS NULL").
		Group("sku.id").
		Having("available <= sku.safe_stock").
		Order("available").Scan(&list).Error
	return list, err
}

// Category 分类销售占比（近 N 天）
func (s *DashboardService) Category(ctx context.Context, days int) ([]CatItem, error) {
	if days <= 0 {
		days = 30
	}
	db := global.GVA_DB.WithContext(ctx)
	from := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var list []CatItem
	err := db.Table("sale_item i").
		Select("COALESCE(c.name,'未分类') AS name, COALESCE(SUM(i.amount),0) AS amount").
		Joins("JOIN sale_order o ON o.id = i.sale_id").
		Joins("JOIN goods_sku sku ON sku.id = i.sku_id").
		Joins("JOIN goods g ON g.id = sku.goods_id").
		Joins("LEFT JOIN goods_category c ON c.id = g.category_id").
		Where("o.status = ? AND o.created_at >= ?", 2, from).
		Group("c.id").Order("amount DESC").Scan(&list).Error
	return list, err
}

// SalesHistory 历史销售统计（日期范围，按日/按月汇总：销售额/订单数/毛利）
func (s *DashboardService) SalesHistory(ctx context.Context, from, to string, granularity string) ([]SalesHistoryItem, error) {
	if from == "" || to == "" {
		return nil, errors.New("请选择日期范围")
	}
	if granularity != "month" {
		granularity = "day"
	}
	db := global.GVA_DB.WithContext(ctx)
	var list []SalesHistoryItem
	err := db.Table("sale_order o").
		Select("DATE(o.created_at) AS date, COALESCE(SUM(o.total_amount),0) AS sales, COUNT(*) AS orders, "+
			"COALESCE(SUM(i.amount - i.qty * COALESCE(sku.cost_price,0) * CASE WHEN i.amount >= 0 THEN 1 ELSE -1 END),0) AS profit").
		Joins("JOIN sale_item i ON i.sale_id = o.id").
		Joins("JOIN goods_sku sku ON sku.id = i.sku_id").
		Where("o.status = ? AND o.created_at >= ? AND o.created_at < ?", 2, from, nextDay(to)).
		Group("DATE(o.created_at)").Order("date").Scan(&list).Error
	if err != nil {
		return nil, err
	}
	// 防御：MySQL parseTime=True 时 DATE 列可能以 RFC3339 形式落到 string（如 2026-08-04T00:00:00+08:00），统一截断
	for i := range list {
		list[i].Date = trimDateT(list[i].Date)
	}
	if granularity == "month" {
		// 按月聚合（Go 侧完成，避免 SQL 方言差异）
		byMonth := map[string]*SalesHistoryItem{}
		var result []SalesHistoryItem
		for i := range list {
			month := list[i].Date
			if len(month) >= 7 {
				month = month[:7]
			}
			if m, ok := byMonth[month]; ok {
				m.Sales += list[i].Sales
				m.Orders += list[i].Orders
				m.Profit += list[i].Profit
			} else {
				byMonth[month] = &SalesHistoryItem{Date: month, Sales: list[i].Sales, Orders: list[i].Orders, Profit: list[i].Profit}
			}
		}
		for _, m := range byMonth {
			result = append(result, *m)
		}
		// 按月排序
		sort.Slice(result, func(a, b int) bool { return result[a].Date < result[b].Date })
		return result, nil
	}
	return list, nil
}

// mondayOf 本周一
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, t.Location())
}

// nextDay 次日（yyyy-mm-dd）
func nextDay(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}
