// Package system 对应 Java 的 scaffold-system 模块：认证、后台账号/角色/菜单/字典/参数、日志、工作台。
package system

import (
	"database/sql"

	"scaffold/internal/framework"
)

// Deps 由 main 组装后传给各 Register*Routes。
type Deps struct {
	DB       *sql.DB
	Security *framework.Security
	Recorder framework.OperationLogRecorder
	// Perm 为 SystemPermissionResolver，SystemManagementService.userOptions 直接用它判断按钮权限。
	Perm *PermissionResolver
}

// IdResponse 对应 Java IdResponse record。
type IdResponse struct {
	ID int64 `json:"id"`
}
