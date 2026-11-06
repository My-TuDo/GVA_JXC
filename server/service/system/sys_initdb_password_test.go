package system

import (
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/model/system/request"
)

// 初始化接口是公开的：管理员口令必须显式提供且达到最小长度，
// 不能依赖历史上 "123456" 这种默认回退值。
func TestInitDBRejectsWeakAdminPassword(t *testing.T) {
	tests := []struct {
		name string
		pwd  string
	}{
		{name: "空密码", pwd: ""},
		{name: "全空白", pwd: "   "},
		{name: "过短", pwd: "1234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := InitDBService{}
			err := svc.InitDB(request.InitDB{AdminPassword: tt.pwd, DBType: "sqlite"})
			if err == nil {
				t.Fatal("弱管理员口令应被拒绝，实际通过")
			}
		})
	}
}
