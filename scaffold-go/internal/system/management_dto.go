package system

// 对应 Java com.scaffold.system.domain.dto 下的写请求。
// Java 装箱字段（String/Integer/Long/Boolean/List<Long>）一律用指针或切片，保留 null 语义。

// PostWriteRequest 对应 Java PostWriteRequest。
type PostWriteRequest struct {
	PostCode *string `json:"postCode"`
	PostName *string `json:"postName"`
	Sort     *int    `json:"sort"`
	Status   *int    `json:"status"`
	Remark   *string `json:"remark"`
}

// UserWriteRequest 对应 Java UserWriteRequest。
type UserWriteRequest struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
	Nickname *string `json:"nickname"`
	Phone    *string `json:"phone"`
	Email    *string `json:"email"`
	DeptID   *int64  `json:"deptId"`
	Status   *int    `json:"status"`
	RoleIDs  []int64 `json:"roleIds"`
	PostIDs  []int64 `json:"postIds"`
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
	Name      *string `json:"name"`
	Code      *string `json:"code"`
	DataScope *string `json:"dataScope"`
	Sort      *int    `json:"sort"`
	Status    *int    `json:"status"`
	Remark    *string `json:"remark"`
	MenuIDs   []int64 `json:"menuIds"`
}

// DeptWriteRequest 对应 Java DeptWriteRequest。
type DeptWriteRequest struct {
	ParentID *int64  `json:"parentId"`
	Name     *string `json:"name"`
	Leader   *string `json:"leader"`
	Phone    *string `json:"phone"`
	Email    *string `json:"email"`
	Sort     *int    `json:"sort"`
	Status   *int    `json:"status"`
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
