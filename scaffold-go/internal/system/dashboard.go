package system

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"scaffold/internal/framework"
)

// DashboardService 工作台：应用 / 应用用户 / 注册 / 登录 / 后台账号概览，趋势、应用明细、最近动态与数值账户流水。
// 契约见 docs/platform-api.md「工作台」；时间边界取数据库时钟（复用 MonitorStatsService.window）。
type DashboardService struct{ db *sql.DB }

func NewDashboardService(db *sql.DB) *DashboardService { return &DashboardService{db: db} }

// javaRound 等价于 Java Math.round(double)：floor(x + 0.5)。
func javaRound(x float64) float64 { return math.Floor(x + 0.5) }

// dashLoginEvents 登录次数口径：成功与失败都算一次尝试；成功 = event 'login' 且 status 1。
const dashLoginEvents = "l.event IN ('login', 'login_failed')"

// dashEventText 认证日志事件 → 动态文案；status = 0 且文案不含「失败」时追加。
var dashEventText = map[string]string{
	"register": "注册", "login": "登录", "logout": "登出", "refresh": "刷新令牌",
	"login_failed": "登录失败", "register_failed": "注册失败",
}

// Summary 对应 GET /api/dashboard/summary?range=24h|7d|30d。非 super_admin 只统计 sys_user_app 绑定的应用。
func (s *DashboardService) Summary(ctx context.Context, rng string) (map[string]any, error) {
	scope, err := selectAppScope(ctx, s.db, framework.CurrentUsername(ctx))
	if err != nil {
		return nil, err
	}
	w, err := NewMonitorStatsService(s.db).window(ctx, rng)
	if err != nil {
		return nil, err
	}
	since, _ := time.Parse(monDBTime, w.since)
	// 上一等长周期：[since - (now - since), since)
	prevSince := since.Add(-w.nowTime.Sub(since)).Format(monDBTime)

	stats, err := s.stats(ctx, scope, w.since, prevSince)
	if err != nil {
		return nil, err
	}
	trend, err := s.trend(ctx, scope, w)
	if err != nil {
		return nil, err
	}
	apps, err := s.apps(ctx, scope, w.since)
	if err != nil {
		return nil, err
	}
	activities, err := s.activities(ctx, scope)
	if err != nil {
		return nil, err
	}
	counters, err := s.counters(ctx, scope, w.since)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"stats":      stats,
		"range":      w.rng,
		"trend":      trend,
		"apps":       apps,
		"activities": activities,
		"counters":   counters,
	}, nil
}

func (s *DashboardService) stats(ctx context.Context, scope appScope, since, prevSince string) ([]map[string]any, error) {
	appClause, appArgs := scope.clause("a.id")
	appRow, err := framework.SelectRow(ctx, s.db, `
		SELECT COUNT(*) AS total, COALESCE(SUM(a.status = 1), 0) AS enabled
		FROM app_info a
		WHERE a.deleted = 0`+appClause, appArgs...)
	if err != nil {
		return nil, err
	}
	userClause, userArgs := scope.clause("u.app_id")
	userRow, err := framework.SelectRow(ctx, s.db, `
		SELECT COUNT(*) AS total,
		       COALESCE(SUM(u.create_time >= ?), 0) AS registrations,
		       COALESCE(SUM(u.create_time >= ? AND u.create_time < ?), 0) AS prevRegistrations
		FROM app_user u
		WHERE u.deleted = 0`+userClause, append([]any{since, prevSince, since}, userArgs...)...)
	if err != nil {
		return nil, err
	}
	loginClause, loginArgs := scope.clause("l.app_id")
	loginRow, err := framework.SelectRow(ctx, s.db, `
		SELECT COALESCE(SUM(l.event_time >= ?), 0) AS total,
		       COALESCE(SUM(l.event_time >= ? AND l.event = 'login' AND l.status = 1), 0) AS success,
		       COALESCE(SUM(l.event_time >= ? AND l.event_time < ?), 0) AS prevTotal
		FROM app_auth_log l
		WHERE `+dashLoginEvents+` AND l.event_time >= ?`+loginClause,
		append([]any{since, since, prevSince, since, prevSince}, loginArgs...)...)
	if err != nil {
		return nil, err
	}
	operRow, err := framework.SelectRow(ctx, s.db, `
		SELECT COUNT(*) AS total, COALESCE(SUM(status = 1), 0) AS enabled
		FROM sys_user
		WHERE deleted = 0`)
	if err != nil {
		return nil, err
	}
	users, registrations, prevRegistrations := monNumber(userRow["total"]), monNumber(userRow["registrations"]), monNumber(userRow["prevRegistrations"])
	logins, loginSuccess, prevLogins := monNumber(loginRow["total"]), monNumber(loginRow["success"]), monNumber(loginRow["prevTotal"])

	usersStat := dashCompare("users", "应用用户", users, users-registrations)
	usersStat["sub"] = dashDeltaText(usersStat)
	registrationsStat := dashCompare("registrations", "新注册", registrations, prevRegistrations)
	registrationsStat["sub"] = "本周期"
	loginsStat := dashCompare("logins", "登录次数", logins, prevLogins)
	loginsStat["sub"] = fmt.Sprintf("成功率 %.1f%%", monSuccessRate(logins, logins-loginSuccess))
	return []map[string]any{
		dashPlain("apps", "应用", monNumber(appRow["total"]), monNumber(appRow["enabled"])),
		usersStat,
		registrationsStat,
		loginsStat,
		dashPlain("operators", "后台账号", monNumber(operRow["total"]), monNumber(operRow["enabled"])),
	}, nil
}

func dashPlain(key, label string, value, enabled int64) map[string]any {
	return map[string]any{"key": key, "label": label, "value": value, "sub": fmt.Sprintf("已启用 %d", enabled)}
}

// dashCompare 环比：delta 为与上一周期的百分比绝对值（保留一位小数），up 为方向；上一周期为 0 时 fresh = true。
func dashCompare(key, label string, current, previous int64) map[string]any {
	stat := map[string]any{"key": key, "label": label, "value": current, "delta": 0.0, "up": current > 0, "fresh": previous <= 0}
	if previous > 0 {
		pct := javaRound(float64(current-previous)*1000.0/float64(previous)) / 10.0
		stat["delta"] = math.Abs(pct)
		stat["up"] = pct >= 0
	}
	return stat
}

func dashDeltaText(stat map[string]any) string {
	if stat["fresh"].(bool) {
		return "上一周期无数据"
	}
	sign := "-"
	if stat["up"].(bool) {
		sign = "+"
	}
	return fmt.Sprintf("较上一周期 %s%.1f%%", sign, stat["delta"].(float64))
}

// trend 按窗口分桶：24h 每小时 24 桶（HH:00），7d / 30d 每天（MM-dd）。
func (s *DashboardService) trend(ctx context.Context, scope appScope, w monWindow) (map[string]any, error) {
	labels := make([]string, 0, w.buckets)
	for i := 0; i < w.buckets; i++ {
		ago := w.buckets - 1 - i
		if w.hourly {
			labels = append(labels, w.nowTime.Add(-time.Duration(ago)*time.Hour).Format("15:00"))
		} else {
			labels = append(labels, startOfDay(w.nowTime).AddDate(0, 0, -ago).Format("01-02"))
		}
	}
	userClause, userArgs := scope.clause("u.app_id")
	registrations, err := s.buckets(ctx, w, "app_user u", "u.create_time", "u.deleted = 0", userClause, userArgs)
	if err != nil {
		return nil, err
	}
	loginClause, loginArgs := scope.clause("l.app_id")
	logins, err := s.buckets(ctx, w, "app_auth_log l", "l.event_time", dashLoginEvents, loginClause, loginArgs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"labels": labels, "registrations": registrations, "logins": logins}, nil
}

// buckets 按「距今小时数 / 天数」聚合后补零，下标 0 为最早的桶。
func (s *DashboardService) buckets(ctx context.Context, w monWindow, table, timeCol, where string, scopeClause string, scopeArgs []any) ([]int64, error) {
	ago := "DATEDIFF(DATE(?), DATE(" + timeCol + "))"
	if w.hourly {
		ago = "TIMESTAMPDIFF(HOUR, " + timeCol + ", ?)"
	}
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT `+ago+` AS ago, COUNT(*) AS total
		FROM `+table+`
		WHERE `+where+` AND `+timeCol+` >= ?`+scopeClause+`
		GROUP BY ago`, append([]any{w.now, w.since}, scopeArgs...)...)
	if err != nil {
		return nil, err
	}
	byAgo := map[int64]int64{}
	for _, row := range rows {
		byAgo[monNumber(row["ago"])] = monNumber(row["total"])
	}
	values := make([]int64, 0, w.buckets)
	for i := 0; i < w.buckets; i++ {
		values = append(values, byAgo[int64(w.buckets-1-i)])
	}
	return values, nil
}

func (s *DashboardService) apps(ctx context.Context, scope appScope, since string) ([]framework.Row, error) {
	clause, args := scope.clause("a.id")
	return framework.SelectRows(ctx, s.db, `
		SELECT a.id,
		       a.app_code AS code,
		       a.app_name AS name,
		       (SELECT COUNT(*) FROM app_user u WHERE u.app_id = a.id AND u.deleted = 0) AS users,
		       (SELECT COUNT(*) FROM app_user u WHERE u.app_id = a.id AND u.deleted = 0 AND u.create_time >= ?) AS registrations,
		       (SELECT COUNT(*) FROM app_auth_log l WHERE l.app_id = a.id AND `+dashLoginEvents+` AND l.event_time >= ?) AS logins
		FROM app_info a
		WHERE a.deleted = 0`+clause+`
		ORDER BY a.id`, append([]any{since, since}, args...)...)
}

// activities 最近 8 条认证日志（注册 / 登录 / 登出 / 刷新及失败），不限时间窗口，只限应用范围。
func (s *DashboardService) activities(ctx context.Context, scope appScope) ([]map[string]any, error) {
	clause, args := scope.clause("l.app_id")
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT a.app_name AS app,
		       COALESCE(l.username, '') AS who,
		       l.event,
		       l.status,
		       DATE_FORMAT(l.event_time, '%Y-%m-%d %H:%i') AS time
		FROM app_auth_log l
		JOIN app_info a ON a.id = l.app_id
		WHERE a.deleted = 0`+clause+`
		ORDER BY l.event_time DESC, l.id DESC
		LIMIT 8`, args...)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		event := framework.StrOrEmpty(row["event"])
		action, ok := dashEventText[event]
		if !ok {
			action = event
		}
		tint := "ok"
		if monNumber(row["status"]) == 0 {
			tint = "danger"
			if !strings.HasSuffix(action, "失败") {
				action += "失败"
			}
		}
		out = append(out, map[string]any{"app": row["app"], "who": row["who"], "action": action, "time": row["time"], "tint": tint})
	}
	return out, nil
}

// counters 窗口内各应用各数值账户的正向发放量（正数 amount 之和）与变动次数（流水行数）。
func (s *DashboardService) counters(ctx context.Context, scope appScope, since string) ([]map[string]any, error) {
	clause, args := scope.clause("c.app_id")
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT a.app_name AS app,
		       c.field_key AS fieldKey,
		       COALESCE(f.label, c.field_key) AS label,
		       COALESCE(SUM(CASE WHEN c.amount > 0 THEN c.amount ELSE 0 END), 0) AS total,
		       COUNT(*) AS changes
		FROM app_counter_log c
		JOIN app_info a ON a.id = c.app_id
		LEFT JOIN app_field f ON f.app_id = c.app_id AND f.field_key = c.field_key
		WHERE a.deleted = 0 AND c.create_time >= ?`+clause+`
		GROUP BY c.app_id, a.app_name, c.field_key, f.label
		ORDER BY total DESC, c.app_id, c.field_key
		LIMIT 10`, append([]any{since}, args...)...)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"app": row["app"], "fieldKey": row["fieldKey"], "label": row["label"],
			"total": framework.AsFloat64(row["total"]), "changes": monNumber(row["changes"]),
		})
	}
	return out, nil
}

// RegisterDashboardRoutes 对应 DashboardController。
func RegisterDashboardRoutes(mux *http.ServeMux, s *DashboardService) {
	mux.Handle("GET /api/dashboard/summary", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		data, err := s.Summary(r.Context(), framework.QueryStringDefault(r, "range", "24h"))
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}))
}
