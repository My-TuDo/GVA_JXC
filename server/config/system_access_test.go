package config

import (
	"testing"

	"github.com/spf13/viper"
)

func boolPtr(v bool) *bool { return &v }

// 指针开关必须能从 YAML 经 viper 正确落到配置结构体：
// 未写 -> nil（走模式默认），写 true/false -> 尊重显式值。
func TestViperUnmarshalsAccessSwitches(t *testing.T) {
	load := func(path string) System {
		v := viper.New()
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		var cfg Server
		if err := v.Unmarshal(&cfg); err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		return cfg.System
	}

	example := load("../config.example.yaml")
	if example.InitDBEnable != nil || example.SwaggerEnable != nil {
		t.Fatalf("示例配置不应写死开关（应走模式默认），实际 init=%v swagger=%v", example.InitDBEnable, example.SwaggerEnable)
	}

	docker := load("../config.docker.yaml")
	if docker.InitDBEnable == nil || !*docker.InitDBEnable {
		t.Fatalf("容器配置应显式开启初始化接口，实际: %v", docker.InitDBEnable)
	}
	if docker.SwaggerEnable == nil || *docker.SwaggerEnable {
		t.Fatalf("容器配置应显式关闭 swagger，实际: %v", docker.SwaggerEnable)
	}
}

// 未显式配置时必须按运行模式取安全默认值：
// 初始化接口能指定任意数据库与管理员账号，release 下绝不能默认敞开。
func TestSystemInitDBEnabled(t *testing.T) {
	tests := []struct {
		name    string
		flag    *bool
		release bool
		want    bool
	}{
		{name: "未配置-debug默认开启", flag: nil, release: false, want: true},
		{name: "未配置-release默认关闭", flag: nil, release: true, want: false},
		{name: "显式开启-release也开启", flag: boolPtr(true), release: true, want: true},
		{name: "显式关闭-debug也关闭", flag: boolPtr(false), release: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := System{InitDBEnable: tt.flag}
			if got := s.InitDBEnabled(tt.release); got != tt.want {
				t.Fatalf("InitDBEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// swagger 同理：debug 方便自测，release 不把接口契约暴露给公网。
func TestSystemSwaggerEnabled(t *testing.T) {
	tests := []struct {
		name    string
		flag    *bool
		release bool
		want    bool
	}{
		{name: "未配置-debug默认开启", flag: nil, release: false, want: true},
		{name: "未配置-release默认关闭", flag: nil, release: true, want: false},
		{name: "显式开启-release也开启", flag: boolPtr(true), release: true, want: true},
		{name: "显式关闭-debug也关闭", flag: boolPtr(false), release: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := System{SwaggerEnable: tt.flag}
			if got := s.SwaggerEnabled(tt.release); got != tt.want {
				t.Fatalf("SwaggerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
