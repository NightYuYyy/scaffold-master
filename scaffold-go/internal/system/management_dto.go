package system

// 对应 Java com.scaffold.system.domain.dto 下的写请求。
// Java 装箱字段（String/Integer/Long/Boolean/List<Long>）一律用指针或切片，保留 null 语义。

// UserWriteRequest 后台账号写请求；AppIDs 为可管理的应用（sys_user_app），超级管理员不受其限制。
type UserWriteRequest struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
	Nickname *string `json:"nickname"`
	Phone    *string `json:"phone"`
	Email    *string `json:"email"`
	Status   *int    `json:"status"`
	RoleIDs  []int64 `json:"roleIds"`
	AppIDs   []int64 `json:"appIds"`
}

// UserStatusRequest 对应 Java UserStatusRequest。
type UserStatusRequest struct {
	Status *int `json:"status"`
}

// UserPasswordResetRequest 对应 Java UserPasswordResetRequest。
type UserPasswordResetRequest struct {
	Password *string `json:"password"`
}

// RoleWriteRequest 对应 Java RoleWriteRequest。
type RoleWriteRequest struct {
	Name    *string `json:"name"`
	Code    *string `json:"code"`
	Sort    *int    `json:"sort"`
	Status  *int    `json:"status"`
	Remark  *string `json:"remark"`
	MenuIDs []int64 `json:"menuIds"`
}

// MenuWriteRequest 对应 Java MenuWriteRequest。
type MenuWriteRequest struct {
	ParentID   *int64  `json:"parentId"`
	Name       *string `json:"name"`
	Type       *string `json:"type"`
	Icon       *string `json:"icon"`
	Path       *string `json:"path"`
	Component  *string `json:"component"`
	Permission *string `json:"permission"`
	Sort       *int    `json:"sort"`
	Visible    *int    `json:"visible"`
	Status     *int    `json:"status"`
}

// DictTypeWriteRequest 对应 Java DictTypeWriteRequest。
type DictTypeWriteRequest struct {
	DictName *string `json:"dictName"`
	DictType *string `json:"dictType"`
	Status   *int    `json:"status"`
	Remark   *string `json:"remark"`
}

// DictDataWriteRequest 对应 Java DictDataWriteRequest。
type DictDataWriteRequest struct {
	DictType *string `json:"dictType"`
	Label    *string `json:"label"`
	Value    *string `json:"value"`
	Sort     *int    `json:"sort"`
	Status   *int    `json:"status"`
	Tone     *string `json:"tone"`
	Def      *bool   `json:"def"`
	Remark   *string `json:"remark"`
}

// ConfigWriteRequest 对应 Java ConfigWriteRequest。
type ConfigWriteRequest struct {
	Name    *string `json:"name"`
	Key     *string `json:"key"`
	Value   *string `json:"value"`
	Type    *string `json:"type"`
	Builtin *bool   `json:"builtin"`
	Remark  *string `json:"remark"`
}
