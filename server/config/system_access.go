package config

// InitDBEnabled 报告是否注册 /init/initdb、/init/checkdb 初始化接口。
//
// 未显式配置（nil）时按运行模式取默认值：release 关闭，debug/test 开启。
// 这两个接口能指定任意数据库地址与管理员密码并在库里建号，一旦裸露到公网，
// 攻击者可在服务连不上库的窗口期把实例指向自己的数据库完成接管。
// 因此默认值必须是「生产关闭」，而不是「默认开启、靠反向代理兜底」。
func (s System) InitDBEnabled(release bool) bool {
	if s.InitDBEnable != nil {
		return *s.InitDBEnable
	}
	return !release
}

// SwaggerEnabled 报告是否注册 /swagger/*any 文档接口。
//
// 未显式配置时 debug/test 开启、release 关闭：文档会暴露全部接口路径、
// 参数结构与响应类型，等于免费给攻击者一份攻击面清单。
func (s System) SwaggerEnabled(release bool) bool {
	if s.SwaggerEnable != nil {
		return *s.SwaggerEnable
	}
	return !release
}
