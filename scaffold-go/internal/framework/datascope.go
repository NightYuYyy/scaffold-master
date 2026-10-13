package framework

import "context"

// DataScopeCriteria 对应 Java DataScopeCriteria。
// SQL 中 #{scope.allScope} / #{scope.userId} / foreach scope.deptIds 直接读取这三个字段。
type DataScopeCriteria struct {
	AllScope bool
	UserID   *int64
	DeptIDs  []int64
}

func DataScopeAll() DataScopeCriteria { return DataScopeCriteria{AllScope: true, DeptIDs: []int64{}} }

func DataScopeSelfOnly(userID *int64) DataScopeCriteria {
	return DataScopeCriteria{UserID: userID, DeptIDs: []int64{}}
}

// DataScopeLimited 去重并保持顺序（LinkedHashSet 语义）。
func DataScopeLimited(userID *int64, deptIDs []int64) DataScopeCriteria {
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(deptIDs))
	for _, id := range deptIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return DataScopeCriteria{UserID: userID, DeptIDs: ids}
}

// DataScopeResolver 对应 Java DataScopeResolver，由 system 包实现。
// Java 中 @DataScope 切面在进入 service 方法前解析一次；Go 中由 service 方法显式调用。
type DataScopeResolver interface {
	Resolve(ctx context.Context, username string) DataScopeCriteria
}
