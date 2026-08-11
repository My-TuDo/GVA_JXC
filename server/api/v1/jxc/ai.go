package jxc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AiApi AI 智能助手接口
type AiApi struct{}

// AiAnalyzeRequest 业绩分析请求
type AiAnalyzeRequest struct {
	Question string `json:"question"` // 问题（可空，默认综合分析）
	Days     int    `json:"days"`     // 统计天数（默认30）
}

// AiRecommendRequest 商品推荐请求
type AiRecommendRequest struct {
	Demand string `json:"demand"` // 客户需求描述
}

// Analyze 销售业绩分析（SSE 流式）
// @Tags     JxcAi
// @Summary  AI 销售业绩分析（流式）
// @Security ApiKeyAuth
// @Accept   application/json
// @Produce  text/event-stream
// @Param    body body AiAnalyzeRequest true "分析请求"
// @Success  200 {string} string "SSE 流：data: <文本增量>，结束 data: [DONE]"
// @Router   /jxc/ai/analyze [post]
func (a *AiApi) Analyze(c *gin.Context) {
	var req AiAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("参数错误："+err.Error(), c)
		return
	}
	resp, err := aiService.AnalyzeSales(c.Request.Context(), req.Question, req.Days)
	if err != nil {
		global.GVA_LOG.Error("AI 业绩分析失败!", zap.Error(err))
		response.FailWithMessage(err.Error(), c)
		return
	}
	startSSE(c)
	forwardLLMStream(c, resp)
}

// Recommend 客户需求商品推荐（先推候选商品事件，再流式推荐语）
// @Tags     JxcAi
// @Summary  AI 商品推荐（结构化条件+候选，再流式推荐语）
// @Security ApiKeyAuth
// @Accept   application/json
// @Produce  text/event-stream
// @Param    body body AiRecommendRequest true "推荐请求"
// @Success  200 {string} string "SSE 流：event: candidates data: {conditions,candidates}；随后 data: <推荐语增量>，结束 data: [DONE]"
// @Router   /jxc/ai/recommend [post]
func (a *AiApi) Recommend(c *gin.Context) {
	var req AiRecommendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("参数错误："+err.Error(), c)
		return
	}
	conditions, candidates, err := aiService.RecommendGoods(c.Request.Context(), req.Demand)
	if err != nil {
		global.GVA_LOG.Error("AI 商品推荐失败!", zap.Error(err))
		response.FailWithMessage(err.Error(), c)
		return
	}
	startSSE(c)
	// 事件一：结构化条件 + 候选商品（前端渲染推荐卡片）
	payload, _ := json.Marshal(map[string]any{
		"conditions": conditions,
		"candidates": candidates,
	})
	writeSSE(c, "candidates", string(payload))
	// 事件二：推荐语流式输出
	resp, err := aiService.RecommendStream(c.Request.Context(), req.Demand, candidates)
	if err != nil {
		global.GVA_LOG.Error("AI 推荐语生成失败!", zap.Error(err))
		writeSSE(c, "error", err.Error())
		writeSSE(c, "data", "[DONE]")
		return
	}
	forwardLLMStream(c, resp)
}

// startSSE 设置 SSE 响应头
func startSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

// writeSSE 写一个 SSE 事件并 flush
func writeSSE(c *gin.Context, event, data string) {
	if event != "" {
		fmt.Fprintf(c.Writer, "event: %s\n", event)
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
}

// forwardLLMStream 解析上游 OpenAI 兼容 SSE 增量，以纯文本增量转发给前端
func forwardLLMStream(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			writeSSE(c, "data", chunk.Choices[0].Delta.Content)
		}
	}
	writeSSE(c, "data", "[DONE]")
}
