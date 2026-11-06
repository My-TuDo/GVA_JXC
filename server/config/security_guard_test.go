package config

import (
	"strings"
	"testing"
)

func TestProductionSecretIssues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantBad bool
	}{
		{name: "空密钥", key: "", wantBad: true},
		{name: "上游默认qmPlus", key: "qmPlus", wantBad: true},
		{name: "大小写变体", key: "QMPLUS", wantBad: true},
		{name: "带空格仍是占位值", key: "  qmPlus  ", wantBad: true},
		{name: "过短密钥", key: "short-key", wantBad: true},
		{name: "随机长密钥放行", key: "Zx8vQ2mN7pL4rT9wK3yB6cF1hJ5sD0aG", wantBad: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Server{JWT: JWT{SigningKey: tt.key}}
			issues := cfg.ProductionSecretIssues()
			if gotBad := len(issues) > 0; gotBad != tt.wantBad {
				t.Fatalf("ProductionSecretIssues() issues = %v, wantBad = %v", issues, tt.wantBad)
			}
			if tt.wantBad && !strings.Contains(issues[0], "signing-key") {
				t.Fatalf("问题描述应指明 jwt.signing-key，实际: %q", issues[0])
			}
		})
	}
}
