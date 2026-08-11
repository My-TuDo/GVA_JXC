package config

// AI AI 智能助手配置
type AI struct {
	LLM LLM `mapstructure:"llm" json:"llm" yaml:"llm"`
}

// LLM LLM 客户端配置（OpenAI 兼容接口，如 DeepSeek）
type LLM struct {
	BaseURL     string  `mapstructure:"base_url" json:"base_url" yaml:"base_url"`          // 兼容端点，如 https://api.deepseek.com
	APIKey      string  `mapstructure:"api_key" json:"api_key" yaml:"api_key"`            // API Key（可留空，用环境变量 GVA_AI_APIKEY 兜底）
	Model       string  `mapstructure:"model" json:"model" yaml:"model"`                  // 模型名，默认 deepseek-chat
	Timeout     int     `mapstructure:"timeout" json:"timeout" yaml:"timeout"`            // 单次请求超时（秒），默认 60
	MaxTokens   int     `mapstructure:"max_tokens" json:"max_tokens" yaml:"max_tokens"`   // 生成上限，默认 2048
	Temperature float64 `mapstructure:"temperature" json:"temperature" yaml:"temperature"` // 采样温度，默认 0.7
}
