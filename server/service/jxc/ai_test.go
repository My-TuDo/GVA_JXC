package jxc

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/internal/testutil"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
)

// mockLLMClient 可编程 mock（实现 LLMClient 接口，记录最近调用）
type mockLLMClient struct {
	chatResult string
	chatErr    error
	streamResp *http.Response
	streamErr  error
	messages   [][]LLMMessage // 最近一次调用的消息
}

func (m *mockLLMClient) Chat(_ context.Context, messages []LLMMessage, _ bool) (string, error) {
	m.messages = append(m.messages, messages)
	return m.chatResult, m.chatErr
}

func (m *mockLLMClient) ChatStream(_ context.Context, messages []LLMMessage) (*http.Response, error) {
	m.messages = append(m.messages, messages)
	return m.streamResp, m.streamErr
}

// aiTestDB AI 模块测试库（基础 + 商品 + 库存 + 销售）
func aiTestDB(t *testing.T) {
	t.Helper()
	testutil.NewMemoryDB(t,
		&jxc.GoodsCategory{}, &jxc.Brand{}, &jxc.Supplier{}, &jxc.Customer{}, &jxc.Warehouse{},
		&jxc.Goods{}, &jxc.GoodsSku{},
		&jxc.Stock{}, &jxc.StockLog{},
		&jxc.SaleOrder{}, &jxc.SaleItem{},
	)
}

// newAiFixture 前置：分类(连衣裙/裤子) + 4 个商品 SKU + 库存 + 销售记录，返回 skuID 映射
func newAiFixture(t *testing.T) {
	t.Helper()
	db := global.GVA_DB
	dress := jxc.GoodsCategory{Code: "FL01", Name: "连衣裙", Status: 1}
	pants := jxc.GoodsCategory{Code: "FL02", Name: "裤子", Status: 1}
	db.Create(&dress)
	db.Create(&pants)
	db.Create(&jxc.Warehouse{Code: "CK001", Name: "总仓", Status: 1})

	// 夏季碎花连衣裙：红色 M，销量高，库存充足
	g1 := jxc.Goods{Code: "SP001", Name: "夏季碎花连衣裙", CategoryID: &dress.ID, Unit: "件", Status: 1}
	db.Create(&g1)
	sku1 := jxc.GoodsSku{GoodsID: g1.ID, SkuCode: "SP001-RED-M", Color: "红色", Size: "M", SalePrice: 129, Status: 1}
	db.Create(&sku1)
	// 针织连衣裙：黑色 L，价格高
	g2 := jxc.Goods{Code: "SP002", Name: "针织连衣裙", CategoryID: &dress.ID, Unit: "件", Status: 1}
	db.Create(&g2)
	sku2 := jxc.GoodsSku{GoodsID: g2.ID, SkuCode: "SP002-BLK-L", Color: "黑色", Size: "L", SalePrice: 259, Status: 1}
	db.Create(&sku2)
	// 牛仔裤：蓝色 29
	g3 := jxc.Goods{Code: "SP003", Name: "直筒牛仔裤", CategoryID: &pants.ID, Unit: "件", Status: 1}
	db.Create(&g3)
	sku3 := jxc.GoodsSku{GoodsID: g3.ID, SkuCode: "SP003-BLU-29", Color: "蓝色", Size: "29", SalePrice: 99, Status: 1}
	db.Create(&sku3)
	// 下架商品（status 有 default:1，Create 时 0 会被省略写入默认值，故创建后显式改回 0）
	g4 := jxc.Goods{Code: "SP004", Name: "过季风衣", Unit: "件", Status: 1}
	db.Create(&g4)
	db.Model(&g4).Update("status", 0)
	db.Create(&jxc.GoodsSku{GoodsID: g4.ID, SkuCode: "SP004-GRY-M", Color: "灰色", Size: "M", SalePrice: 399, Status: 1})

	db.Create(&jxc.Stock{SkuID: sku1.ID, WarehouseID: 1, Quantity: 20, LockQuantity: 2})
	db.Create(&jxc.Stock{SkuID: sku2.ID, WarehouseID: 1, Quantity: 8, LockQuantity: 0})
	db.Create(&jxc.Stock{SkuID: sku3.ID, WarehouseID: 1, Quantity: 5, LockQuantity: 0})

	// 已出库销售单（status=2）：连衣裙 2 件、牛仔裤 1 件
	order := jxc.SaleOrder{WarehouseID: 1, OrderType: jxc.SaleTypeNormal, Status: jxc.SaleStatusShipped}
	db.Create(&order)
	db.Create(&jxc.SaleItem{SaleID: order.ID, SkuID: sku1.ID, SkuCode: sku1.SkuCode, GoodsName: g1.Name, Qty: 2, Price: 129, Amount: 258, Direction: 0})
	db.Create(&jxc.SaleItem{SaleID: order.ID, SkuID: sku3.ID, SkuCode: sku3.SkuCode, GoodsName: g3.Name, Qty: 1, Price: 99, Amount: 99, Direction: 0})
}

// ========== 条件解析（纯函数） ==========

// TestParseRecommendConditions 正常 JSON
func TestParseRecommendConditions(t *testing.T) {
	cond, err := parseRecommendConditions(`{"keywords":["连衣裙","微胖"],"categoryNames":["连衣裙"],"priceMin":50,"priceMax":200,"color":"红色","size":"M"}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(cond.Keywords) != 2 || cond.Keywords[0] != "连衣裙" {
		t.Errorf("keywords 解析错误: %+v", cond.Keywords)
	}
	if len(cond.CategoryNames) != 1 || cond.CategoryNames[0] != "连衣裙" {
		t.Errorf("categoryNames 解析错误: %+v", cond.CategoryNames)
	}
	if cond.PriceMin == nil || *cond.PriceMin != 50 || cond.PriceMax == nil || *cond.PriceMax != 200 {
		t.Errorf("价格区间解析错误: min=%v max=%v", cond.PriceMin, cond.PriceMax)
	}
	if cond.Color != "红色" || cond.Size != "M" {
		t.Errorf("颜色/尺码解析错误: color=%q size=%q", cond.Color, cond.Size)
	}
}

// TestParseRecommendConditions_CodeFence 兼容 ```json 代码块围栏
func TestParseRecommendConditions_CodeFence(t *testing.T) {
	cond, err := parseRecommendConditions("```json\n{\"keywords\": [\"T恤\"], \"categoryNames\": [], \"priceMin\": null, \"priceMax\": null, \"color\": \"\", \"size\": \"\"}\n```")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(cond.Keywords) != 1 || cond.Keywords[0] != "T恤" {
		t.Errorf("代码块围栏内 keywords 解析错误: %+v", cond.Keywords)
	}
	if cond.PriceMin != nil || cond.PriceMax != nil {
		t.Errorf("null 价格应解析为 nil")
	}
}

// TestParseRecommendConditions_StringPrice 价格可能是字符串
func TestParseRecommendConditions_StringPrice(t *testing.T) {
	cond, err := parseRecommendConditions(`{"keywords":[],"categoryNames":[],"priceMin":"50","priceMax":"100","color":"","size":""}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cond.PriceMin == nil || *cond.PriceMin != 50 || cond.PriceMax == nil || *cond.PriceMax != 100 {
		t.Errorf("字符串价格应解析为数字: min=%v max=%v", cond.PriceMin, cond.PriceMax)
	}
}

// TestParseRecommendConditions_Invalid 非法输入返回错误（由调用方降级）
func TestParseRecommendConditions_Invalid(t *testing.T) {
	if _, err := parseRecommendConditions("not-json"); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// ========== 候选检索 ==========

// TestSearchCandidates 条件过滤 + 销量排序 + 上架过滤
func TestSearchCandidates(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	svc := &AiService{client: &mockLLMClient{}}

	list, err := svc.searchCandidates(ctx, RecommendConditions{Keywords: []string{"连衣裙"}})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("连衣裙应命中 2 个 SKU, got %d: %+v", len(list), list)
	}
	// 销量降序：碎花连衣裙(2件) 应排在针织连衣裙(0件) 前
	if list[0].SkuCode != "SP001-RED-M" {
		t.Errorf("销量降序排序错误: %+v", list)
	}
	if list[0].Available != 18 {
		t.Errorf("可售库存计算错误: %+v", list[0])
	}
}

// TestSearchCandidates_Filters 分类/价格/颜色/尺码组合过滤
func TestSearchCandidates_Filters(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	svc := &AiService{client: &mockLLMClient{}}

	min, max := 100.0, 300.0
	list, err := svc.searchCandidates(ctx, RecommendConditions{
		CategoryNames: []string{"连衣裙"},
		PriceMin:      &min,
		PriceMax:      &max,
		Color:         "黑",
		Size:          "L",
	})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(list) != 1 || list[0].SkuCode != "SP002-BLK-L" {
		t.Fatalf("组合过滤应命中 SP002-BLK-L: %+v", list)
	}
}

// TestSearchCandidates_Empty 无条件检索返回全部上架 SKU（销量降序）
func TestSearchCandidates_Empty(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	svc := &AiService{client: &mockLLMClient{}}

	list, err := svc.searchCandidates(ctx, RecommendConditions{})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	// 3 个上架 SKU，下架商品 SP004 不出现
	if len(list) != 3 {
		t.Fatalf("应命中 3 个上架 SKU, got %d: %+v", len(list), list)
	}
}

// ========== 推荐 ==========

// TestRecommendGoods mock LLM 返回结构化条件 → 条件+候选正确
func TestRecommendGoods(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	mock := &mockLLMClient{chatResult: `{"keywords":["连衣裙"],"categoryNames":[],"priceMin":null,"priceMax":null,"color":"","size":""}`}
	svc := &AiService{client: mock}

	cond, candidates, err := svc.RecommendGoods(ctx, "想给微胖女性推荐夏天连衣裙")
	if err != nil {
		t.Fatalf("推荐失败: %v", err)
	}
	if len(cond.Keywords) != 1 || cond.Keywords[0] != "连衣裙" {
		t.Errorf("条件解析错误: %+v", cond)
	}
	if len(candidates) != 2 {
		t.Errorf("候选应为 2 个连衣裙 SKU: %+v", candidates)
	}
}

// TestRecommendGoods_InvalidJSON mock 返回非法 JSON → 降级为空条件不报错
func TestRecommendGoods_InvalidJSON(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	mock := &mockLLMClient{chatResult: "抱歉我无法解析"}
	svc := &AiService{client: mock}

	_, candidates, err := svc.RecommendGoods(ctx, "随便推荐点")
	if err != nil {
		t.Fatalf("非法 JSON 应降级而非报错: %v", err)
	}
	if len(candidates) != 3 {
		t.Errorf("降级应返回全部上架 SKU: %+v", candidates)
	}
}

// TestRecommendGoods_EmptyDemand 空需求报错
func TestRecommendGoods_EmptyDemand(t *testing.T) {
	svc := &AiService{client: &mockLLMClient{}}
	if _, _, err := svc.RecommendGoods(ctx, "  "); err == nil {
		t.Fatal("空需求应报错")
	}
}

// TestRecommendGoods_LLMError LLM 调用失败透传错误
func TestRecommendGoods_LLMError(t *testing.T) {
	mock := &mockLLMClient{chatErr: context.DeadlineExceeded}
	svc := &AiService{client: mock}
	if _, _, err := svc.RecommendGoods(ctx, "推荐连衣裙"); err == nil {
		t.Fatal("LLM 失败应透传错误")
	}
}

// ========== 业绩分析（流式） ==========

// TestAnalyzeSales 验证系统提示词三层结构（角色/硬约束/数据）与默认问题
func TestAnalyzeSales(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	mock := &mockLLMClient{streamResp: &http.Response{}}
	svc := &AiService{client: mock}

	resp, err := svc.AnalyzeSales(ctx, "", 0)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if resp != mock.streamResp {
		t.Error("应透传上游 SSE 响应")
	}
	if len(mock.messages) != 1 || len(mock.messages[0]) != 2 {
		t.Fatalf("应发送 system+user 两条消息")
	}
	sys := mock.messages[0][0].Content
	if !strings.Contains(sys, "[角色]") || !strings.Contains(sys, "[硬约束]") || !strings.Contains(sys, "<data>") {
		t.Errorf("系统提示词应包含三层结构: %s", sys[:200])
	}
	user := mock.messages[0][1].Content
	if user != "请综合分析最近的销售情况，指出亮点、问题与经营建议" {
		t.Errorf("空问题应使用默认问题, got %q", user)
	}
}

// TestAnalyzeSales_HasData 数据注入（聚合指标应出现在 <data> 中）
func TestAnalyzeSales_HasData(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	mock := &mockLLMClient{streamResp: &http.Response{}}
	svc := &AiService{client: mock}

	if _, err := svc.AnalyzeSales(ctx, "本月卖得怎么样？", 0); err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	sys := mock.messages[0][0].Content
	// <data> 中应包含聚合字段名
	for _, key := range []string{"overview", "trend", "top", "category", "salesHistory90d", "stockAlert", "range", "generatedAt"} {
		if !strings.Contains(sys, key) {
			t.Errorf("<data> 应包含 %q", key)
		}
	}
	user := mock.messages[0][1].Content
	if !strings.Contains(user, "本月卖得怎么样？") {
		t.Errorf("用户问题应透传, got %q", user)
	}
}

// ========== 时间范围解析（纯函数） ==========

// TestParseTimeRange 上月
func TestParseTimeRange_PrevMonth(t *testing.T) {
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastOfPrev := firstOfMonth.AddDate(0, 0, -1)
	firstOfPrev := time.Date(lastOfPrev.Year(), lastOfPrev.Month(), 1, 0, 0, 0, 0, now.Location())

	from, to, granularity := parseTimeRange("上个月卖得怎么样？")
	if granularity != "month" {
		t.Errorf("上月应为月粒度, got %s", granularity)
	}
	if from != firstOfPrev.Format("2006-01-02") || to != lastOfPrev.Format("2006-01-02") {
		t.Errorf("上月范围错误: %s ~ %s, 期望 %s ~ %s", from, to, firstOfPrev.Format("2006-01-02"), lastOfPrev.Format("2006-01-02"))
	}
}

// TestParseTimeRange_Year 今年/去年
func TestParseTimeRange_Year(t *testing.T) {
	now := time.Now()
	from, _, granularity := parseTimeRange("今年的销售情况如何？")
	if from != time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02") || granularity != "month" {
		t.Errorf("今年范围错误: %s (%s)", from, granularity)
	}
	from2, to2, _ := parseTimeRange("去年对比今年怎么样")
	lastYear := time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, now.Location())
	if from2 != lastYear.Format("2006-01-02") || to2 != lastYear.AddDate(0, 0, 364).Format("2006-01-02") {
		t.Errorf("去年范围错误: %s ~ %s", from2, to2)
	}
}

// TestParseTimeRange_RecentDays 近7天/近90天
func TestParseTimeRange_RecentDays(t *testing.T) {
	now := time.Now()
	from, _, granularity := parseTimeRange("近7天卖了多少？")
	if granularity != "day" {
		t.Errorf("近7天应为日粒度, got %s", granularity)
	}
	if from != now.AddDate(0, 0, -6).Format("2006-01-02") {
		t.Errorf("近7天起点错误: %s", from)
	}
	from90, _, granularity := parseTimeRange("近90天趋势")
	if granularity != "month" || from90 != now.AddDate(0, 0, -89).Format("2006-01-02") {
		t.Errorf("近90天范围错误: %s (%s)", from90, granularity)
	}
}

// TestParseTimeRange_Default 无时间词默认近30天
func TestParseTimeRange_Default(t *testing.T) {
	now := time.Now()
	from, to, granularity := parseTimeRange("分析一下业绩")
	if granularity != "day" {
		t.Errorf("默认应为日粒度, got %s", granularity)
	}
	if from != now.AddDate(0, 0, -29).Format("2006-01-02") || to != now.Format("2006-01-02") {
		t.Errorf("默认范围错误: %s ~ %s", from, to)
	}
}

// TestParseTimeRange_ThisMonth 本月（含"这个月"）
func TestParseTimeRange_ThisMonth(t *testing.T) {
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	from, to, granularity := parseTimeRange("这个月生意怎么样")
	if granularity != "month" || from != firstOfMonth || to != now.Format("2006-01-02") {
		t.Errorf("本月范围错误: %s ~ %s (%s)", from, to, granularity)
	}
}

// TestRecommendStream 验证推荐语生成提示词包含候选数据与硬约束
func TestRecommendStream(t *testing.T) {
	aiTestDB(t)
	newAiFixture(t)
	mock := &mockLLMClient{streamResp: &http.Response{}}
	svc := &AiService{client: mock}

	candidates := []RecommendCandidate{{SkuID: 1, SkuCode: "SP001-RED-M", GoodsName: "夏季碎花连衣裙", SalePrice: 129}}
	resp, err := svc.RecommendStream(ctx, "想给微胖女性推荐连衣裙", candidates)
	if err != nil {
		t.Fatalf("推荐语生成失败: %v", err)
	}
	if resp != mock.streamResp {
		t.Error("应透传上游 SSE 响应")
	}
	sys := mock.messages[0][0].Content
	if !strings.Contains(sys, "[硬约束]") || !strings.Contains(sys, "SP001-RED-M") {
		t.Errorf("推荐提示词应含硬约束与候选数据: %s", sys[:200])
	}
}
