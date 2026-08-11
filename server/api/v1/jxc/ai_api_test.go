package jxc

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ginTestCtx 构造测试 gin.Context（httptest 输出）
func ginTestCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

// TestWriteSSE_DefaultMessage 文本增量不写 event 行（默认 message 事件）
// 回归：曾误传 event="data"，导致前端把文本流识别为命名事件而丢弃（无输出）
func TestWriteSSE_DefaultMessage(t *testing.T) {
	c, w := ginTestCtx()
	writeSSE(c, "", "你好，这是分析结果")
	body := w.Body.String()
	if strings.Contains(body, "event:") {
		t.Errorf("文本增量不应含 event 行: %q", body)
	}
	if !strings.Contains(body, "data: 你好，这是分析结果") {
		t.Errorf("应输出 data 行: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("事件应以空行结束: %q", body)
	}
}

// TestWriteSSE_NamedEvent 命名事件（candidates/error）保留 event 行
func TestWriteSSE_NamedEvent(t *testing.T) {
	c, w := ginTestCtx()
	writeSSE(c, "candidates", `{"candidates":[]}`)
	body := w.Body.String()
	if !strings.Contains(body, "event: candidates") {
		t.Errorf("命名事件应含 event 行: %q", body)
	}
	if !strings.Contains(body, "data: {\"candidates\":[]}") {
		t.Errorf("命名事件应含 data 行: %q", body)
	}
}

// TestWriteSSE_Done 结束标记 [DONE] 使用默认事件
func TestWriteSSE_Done(t *testing.T) {
	c, w := ginTestCtx()
	writeSSE(c, "", "[DONE]")
	if !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Errorf("结束标记输出错误: %q", w.Body.String())
	}
}
