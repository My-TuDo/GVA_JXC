package config

type System struct {
	DbType             string `mapstructure:"db-type" json:"db-type" yaml:"db-type"`    // 数据库类型:mysql(默认)|sqlite|sqlserver|postgresql
	OssType            string `mapstructure:"oss-type" json:"oss-type" yaml:"oss-type"` // Oss类型
	RouterPrefix       string `mapstructure:"router-prefix" json:"router-prefix" yaml:"router-prefix"`
	Addr               int    `mapstructure:"addr" json:"addr" yaml:"addr"` // 端口值
	LimitCountIP       int    `mapstructure:"iplimit-count" json:"iplimit-count" yaml:"iplimit-count"`
	LimitTimeIP        int    `mapstructure:"iplimit-time" json:"iplimit-time" yaml:"iplimit-time"`
	UseMultipoint      bool   `mapstructure:"use-multipoint" json:"use-multipoint" yaml:"use-multipoint"`                   // 多点登录拦截
	UseRedis           bool   `mapstructure:"use-redis" json:"use-redis" yaml:"use-redis"`                                  // 使用redis
	UseMongo           bool   `mapstructure:"use-mongo" json:"use-mongo" yaml:"use-mongo"`                                  // 使用mongo
	UseStrictAuth      bool   `mapstructure:"use-strict-auth" json:"use-strict-auth" yaml:"use-strict-auth"`                // 使用树形角色分配模式
	DisableAutoMigrate bool   `mapstructure:"disable-auto-migrate" json:"disable-auto-migrate" yaml:"disable-auto-migrate"` // 自动迁移数据库表结构，生产环境建议设为false，手动迁移

	// InitDBEnable / SwaggerEnable 用指针区分「未配置」和「显式 false」：
	// 未配置时按 gin 模式取安全默认值（debug 开、release 关），见 system_access.go。
	InitDBEnable  *bool `mapstructure:"init-db-enable" json:"init-db-enable" yaml:"init-db-enable"` // 是否注册 /init 初始化接口
	SwaggerEnable *bool `mapstructure:"swagger-enable" json:"swagger-enable" yaml:"swagger-enable"` // 是否注册 /swagger 文档接口
}
