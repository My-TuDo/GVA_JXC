package config

import (
	"bytes"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestShippedServerConfigsMatchSchema(t *testing.T) {
	type configCase struct {
		name       string
		path       string
		kubernetes bool
	}
	tests := []configCase{
		{name: "example", path: "../config.example.yaml"},
		{name: "docker", path: "../config.docker.yaml"},
		{name: "kubernetes", path: "../../deploy/kubernetes/server/gva-server-configmap.yaml", kubernetes: true},
	}
	// 本地开发可以额外校验自己的 config.yaml；该文件不入库，CI 里不存在。
	if _, err := os.Stat("../config.yaml"); err == nil {
		tests = append(tests, configCase{name: "local", path: "../config.yaml"})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if tt.kubernetes {
				var manifest struct {
					Data map[string]string `yaml:"data"`
				}
				if err := yaml.Unmarshal(content, &manifest); err != nil {
					t.Fatal(err)
				}
				embedded, ok := manifest.Data["config.yaml"]
				if !ok || len(bytes.TrimSpace([]byte(embedded))) == 0 {
					t.Fatal("Kubernetes ConfigMap 缺少非空 data.config.yaml")
				}
				content = []byte(embedded)
			}

			decoder := yaml.NewDecoder(bytes.NewReader(content))
			decoder.KnownFields(true)
			var cfg Server
			if err := decoder.Decode(&cfg); err != nil {
				t.Fatalf("配置模板与 config.Server 不一致: %v", err)
			}
		})
	}
}
