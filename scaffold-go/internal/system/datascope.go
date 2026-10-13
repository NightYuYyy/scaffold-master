package system

import (
	"context"
	"database/sql"
	"strings"

	"scaffold/internal/framework"
)

// DataScopeResolver 对应 Java SystemDataScopeResolver。
type DataScopeResolver struct{ db *sql.DB }

func NewDataScopeResolver(db *sql.DB) *DataScopeResolver { return &DataScopeResolver{db: db} }

func (r *DataScopeResolver) Resolve(ctx context.Context, username string) framework.DataScopeCriteria {
	if strings.TrimSpace(username) == "" {
		return framework.DataScopeSelfOnly(nil)
	}
	user, err := framework.SelectRow(ctx, r.db, `
		SELECT u.id, u.dept_id AS deptId
		FROM sys_user u
		WHERE u.username = ? AND u.deleted = 0 AND u.status = 1
		LIMIT 1`, username)
	if err != nil || user == nil || user["id"] == nil {
		return framework.DataScopeSelfOnly(nil)
	}
	userID := framework.AsInt64Ptr(user["id"])
	deptID := framework.AsInt64Ptr(user["deptId"])
	roles, err := framework.SelectRows(ctx, r.db, `
		SELECT r.id, r.data_scope AS dataScope
		FROM sys_user_role ur
		JOIN sys_role r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.status = 1 AND r.deleted = 0
		ORDER BY r.sort, r.id`, *userID)
	if err != nil {
		return framework.DataScopeSelfOnly(userID)
	}
	for _, role := range roles {
		if framework.StrOrEmpty(role["dataScope"]) == "ALL" {
			return framework.DataScopeAll()
		}
	}
	deptIDs := []int64{}
	for _, role := range roles {
		scope := framework.AsString(role["dataScope"])
		roleID := framework.AsInt64Ptr(role["id"])
		switch {
		case scope == "DEPT" && deptID != nil:
			deptIDs = append(deptIDs, *deptID)
		case scope == "DEPT_AND_CHILD" && deptID != nil:
			ids, _ := framework.SelectInt64s(ctx, r.db, `
				SELECT d.id
				FROM sys_dept d
				WHERE d.deleted = 0
				  AND (d.id = ? OR FIND_IN_SET(?, d.ancestors))
				ORDER BY d.id`, *deptID, *deptID)
			deptIDs = append(deptIDs, ids...)
		case scope == "CUSTOM" && roleID != nil:
			ids, _ := framework.SelectInt64s(ctx, r.db, `SELECT dept_id FROM sys_role_dept WHERE role_id = ? ORDER BY dept_id`, *roleID)
			deptIDs = append(deptIDs, ids...)
		}
	}
	return framework.DataScopeLimited(userID, deptIDs)
}
