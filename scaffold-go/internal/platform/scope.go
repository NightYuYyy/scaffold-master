package platform

import (
	"context"
	"net/http"
	"slices"

	"scaffold/internal/framework"
)

// appScope 后台账号的应用范围：super_admin 可见全部，其余只看 sys_user_app 绑定的应用。
// 解析委托给 system.PermissionResolver.AppScope，与后台账号模块共用同一套规则。
type appScope struct {
	all bool
	ids []int64
}

func (sc appScope) visible(id int64) bool { return sc.all || slices.Contains(sc.ids, id) }

// clause 生成列表过滤片段（含前导 AND）；无可见应用时恒为假。
func (sc appScope) clause(col string) (string, []any) {
	if sc.all {
		return "", nil
	}
	if len(sc.ids) == 0 {
		return " AND 1 = 0", nil
	}
	return " AND " + col + " IN (" + framework.In(len(sc.ids)) + ")", framework.Int64Args(sc.ids)
}

func (s *svc) scope(ctx context.Context) (appScope, error) {
	all, ids, err := s.perm.AppScope(ctx, framework.CurrentUsername(ctx))
	return appScope{all: all, ids: ids}, err
}

func (s *svc) isSuperAdmin(ctx context.Context) (bool, error) {
	sc, err := s.scope(ctx)
	return sc.all, err
}

// requireApp 应用不在当前账号范围内 → 403。
func (s *svc) requireApp(ctx context.Context, appID int64) error {
	sc, err := s.scope(ctx)
	if err != nil {
		return err
	}
	if !sc.visible(appID) {
		return framework.ErrForbidden
	}
	return nil
}

// resolveAppID 读取 appId 参数并校验范围；缺省取可见应用中 ID 最小的一个（没有则为 0，列表自然为空）。
func (s *svc) resolveAppID(r *http.Request) (int64, error) {
	id, err := framework.QueryInt64(r, "appId")
	if err != nil {
		return 0, err
	}
	sc, err := s.scope(r.Context())
	if err != nil {
		return 0, err
	}
	if id != nil {
		if !sc.visible(*id) {
			return 0, framework.ErrForbidden
		}
		return *id, nil
	}
	clause, args := sc.clause("a.id")
	return framework.SelectInt64(r.Context(), s.db, `SELECT MIN(a.id) FROM app_info a WHERE a.deleted = 0`+clause, args...)
}
