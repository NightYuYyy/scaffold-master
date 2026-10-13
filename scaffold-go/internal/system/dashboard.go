package system

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"time"

	"scaffold/internal/framework"
)

// DashboardService 对应 Java DashboardService + DashboardMapper。
type DashboardService struct{ db *sql.DB }

func NewDashboardService(db *sql.DB) *DashboardService { return &DashboardService{db: db} }

// javaRound 等价于 Java Math.round(double)：floor(x + 0.5)。
func javaRound(x float64) float64 { return math.Floor(x + 0.5) }

func (s *DashboardService) count(ctx context.Context, query string, args ...any) (int64, error) {
	return framework.SelectInt64(ctx, s.db, query, args...)
}

// Summary 对应 DashboardService.summary。
func (s *DashboardService) Summary(ctx context.Context, rng string) (map[string]any, error) {
	hours := 24
	switch rng {
	case "7d":
		hours = 168
	case "30d":
		hours = 720
	}
	stats, err := s.stats(ctx)
	if err != nil {
		return nil, err
	}
	trend, err := s.trend(ctx, hours)
	if err != nil {
		return nil, err
	}
	var trendPrev int64
	if hours <= 24 {
		// countLoginsPrev24h
		trendPrev, err = s.count(ctx, `SELECT COUNT(*) FROM sys_login_log WHERE login_time >= DATE_SUB(NOW(), INTERVAL 48 HOUR) AND login_time < DATE_SUB(NOW(), INTERVAL 24 HOUR)`)
	} else {
		// countLoginsPrevDays
		trendPrev, err = s.count(ctx, `
			SELECT COUNT(*)
			FROM sys_login_log
			WHERE login_time >= CURDATE() - INTERVAL (? * 2 - 1) DAY
			  AND login_time < CURDATE() - INTERVAL (? - 1) DAY`, hours/24, hours/24)
	}
	if err != nil {
		return nil, err
	}
	dailyOps, err := s.dailyOps(ctx)
	if err != nil {
		return nil, err
	}
	// selectActivities
	activities, err := framework.SelectRows(ctx, s.db, `
		SELECT username AS who,
		       CASE status WHEN 1 THEN '登录系统' ELSE '登录失败' END AS action,
		       COALESCE(msg, '登录') AS target,
		       DATE_FORMAT(login_time, '%Y-%m-%d %H:%i') AS time,
		       CASE status WHEN 1 THEN 'ok' ELSE 'danger' END AS tint
		FROM sys_login_log
		ORDER BY login_time DESC, id DESC
		LIMIT 8`)
	if err != nil {
		return nil, err
	}
	// selectRoleDistribution
	roleDist, err := framework.SelectRows(ctx, s.db, `
		SELECT r.data_scope AS label, COUNT(u.id) AS value
		FROM sys_role r
		LEFT JOIN sys_user_role ur ON r.id = ur.role_id
		LEFT JOIN sys_user u ON u.id = ur.user_id AND u.deleted = 0
		WHERE r.deleted = 0
		GROUP BY r.data_scope
		ORDER BY value DESC`)
	if err != nil {
		return nil, err
	}
	// countApiCallsInHours / countApiCallsSuccessInHours
	totalCalls, err := s.count(ctx, `SELECT COUNT(*) FROM sys_oper_log WHERE oper_time >= DATE_SUB(NOW(), INTERVAL ? HOUR)`, hours)
	if err != nil {
		return nil, err
	}
	successCalls, err := s.count(ctx, `SELECT COUNT(*) FROM sys_oper_log WHERE oper_time >= DATE_SUB(NOW(), INTERVAL ? HOUR) AND result = 1`, hours)
	if err != nil {
		return nil, err
	}
	successRate := 100.0
	if totalCalls > 0 {
		successRate = javaRound(float64(successCalls)*1000.0/float64(totalCalls)) / 10.0
	}
	return map[string]any{
		"stats":       stats,
		"trend":       trend,
		"trendPrev":   trendPrev,
		"dailyOps":    dailyOps,
		"activities":  activities,
		"roleDist":    roleDist,
		"trendTotal":  totalCalls,
		"successRate": successRate,
	}, nil
}

// dailyOps 近 7 天按日操作量，缺失的日期补 0；日期取数据库当天。
func (s *DashboardService) dailyOps(ctx context.Context) ([]map[string]any, error) {
	// selectDailyOps
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT DATEDIFF(DATE(oper_time), CURDATE() - INTERVAL 6 DAY) AS dayIndex,
		       COUNT(*) AS total,
		       COALESCE(SUM(result = 0), 0) AS failed
		FROM sys_oper_log
		WHERE oper_time >= CURDATE() - INTERVAL 6 DAY
		GROUP BY dayIndex`)
	if err != nil {
		return nil, err
	}
	byDay := map[int64]framework.Row{}
	for _, row := range rows {
		if row["dayIndex"] != nil {
			byDay[framework.AsInt64(row["dayIndex"])] = row
		}
	}
	// selectToday
	todayRow, err := framework.SelectRow(ctx, s.db, `SELECT DATE_FORMAT(CURDATE(), '%Y-%m-%d') AS today`)
	if err != nil {
		return nil, err
	}
	today, err := time.Parse("2006-01-02", framework.StrOrEmpty(todayRow["today"]))
	if err != nil {
		return nil, err
	}
	days := make([]map[string]any, 0, 7)
	for i := 0; i < 7; i++ {
		row := byDay[int64(i)]
		day := map[string]any{"date": today.AddDate(0, 0, -(6 - i)).Format("2006-01-02"), "total": int64(0), "failed": int64(0)}
		if row != nil {
			day["total"] = framework.AsInt64(row["total"])
			day["failed"] = framework.AsInt64(row["failed"])
		}
		days = append(days, day)
	}
	return days, nil
}

func (s *DashboardService) stats(ctx context.Context) ([]map[string]any, error) {
	queries := []string{
		`SELECT COUNT(*) FROM sys_user WHERE deleted = 0`,                                                                                          // countUsers
		`SELECT COUNT(*) FROM sys_login_log WHERE status = 1 AND login_time >= DATE_SUB(NOW(), INTERVAL 1 HOUR)`,                                   // countOnlineUsers
		`SELECT COUNT(*) FROM sys_role WHERE deleted = 0`,                                                                                          // countRoles
		`SELECT COUNT(*) FROM sys_oper_log WHERE oper_time >= DATE_SUB(NOW(), INTERVAL 24 HOUR)`,                                                   // countApiCalls
		`SELECT COUNT(*) FROM sys_user WHERE deleted = 0 AND create_time >= DATE_SUB(NOW(), INTERVAL 1 DAY)`,                                       // countUsersCreatedSince1d
		`SELECT COUNT(*) FROM sys_login_log WHERE status = 1 AND login_time >= DATE_SUB(NOW(), INTERVAL 2 HOUR)`,                                   // countOnlineUsers2h
		`SELECT COUNT(*) FROM sys_role WHERE deleted = 0 AND create_time >= DATE_SUB(NOW(), INTERVAL 1 DAY)`,                                       // countRolesCreatedSince1d
		`SELECT COUNT(*) FROM sys_oper_log WHERE oper_time >= DATE_SUB(NOW(), INTERVAL 48 HOUR) AND oper_time < DATE_SUB(NOW(), INTERVAL 24 HOUR)`, // countApiCallsPrev24h
		`SELECT COUNT(*) FROM sys_menu`,                   // countMenus
		`SELECT COUNT(*) FROM sys_dept WHERE deleted = 0`, // countDepts
	}
	v := make([]int64, len(queries))
	for i, q := range queries {
		n, err := s.count(ctx, q)
		if err != nil {
			return nil, err
		}
		v[i] = n
	}
	users, online, roles, apiCalls, usersNew, online2h, rolesNew, apiPrev, menus, depts := v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9]
	return []map[string]any{
		dashStat("users", "用户总数", users, "users", "ok", dashDelta(users, usersNew)),
		dashStat("online", "在线用户", online, "monitor", "info", dashDelta(online, online2h)),
		dashStat("roles", "角色数量", roles, "shield", "warn", dashDelta(roles, rolesNew)),
		dashStat("api", "24h 接口调用", apiCalls, "command", "purple", dashDeltaCompare(apiCalls, apiPrev)),
		dashStat("menus", "菜单项", menus, "menu", "info", dashDelta(1, 0)),
		dashStat("depts", "部门数", depts, "dept", "ok", dashDelta(1, 0)),
	}, nil
}

func dashStat(key, label string, value int64, icon, tone string, delta []float64) map[string]any {
	return map[string]any{
		"key":   key,
		"label": label,
		"value": value,
		"icon":  icon,
		"tone":  tone,
		"delta": delta[0],
		"up":    delta[1] > 0,
		"fresh": len(delta) > 2 && delta[2] > 0,
		"sub":   dashSubText(key),
	}
}

func dashDelta(total, recent int64) []float64 {
	if total == 0 {
		return []float64{0, 0}
	}
	pct := javaRound(float64(recent)*1000.0/float64(max(total, 1))) / 10.0
	return []float64{pct, 1}
}

func dashDeltaCompare(current, previous int64) []float64 {
	// 上一周期为 0 时无法计算环比，标记为「新增」（第三位），前端不再显示误导的 +100%
	if previous == 0 {
		flag := 0.0
		if current > 0 {
			flag = 1
		}
		return []float64{0, flag, flag}
	}
	pct := javaRound(float64(current-previous)*1000.0/float64(previous)) / 10.0
	dir := -1.0
	if pct >= 0 {
		dir = 1
	}
	return []float64{math.Abs(pct), dir}
}

func dashSubText(key string) string {
	switch key {
	case "users":
		return "当前启用账号"
	case "online":
		return "最近 1 小时登录"
	case "roles":
		return "已启用角色"
	case "api":
		return "最近 24 小时操作"
	case "menus":
		return "目录 / 菜单 / 按钮"
	case "depts":
		return "未删除部门"
	}
	return ""
}

func (s *DashboardService) trend(ctx context.Context, hours int) ([]int64, error) {
	totals := map[int64]int64{}
	if hours <= 24 {
		// selectLoginTrend
		rows, err := framework.SelectRows(ctx, s.db, `
			SELECT HOUR(login_time) AS hourValue, COUNT(*) AS total
			FROM sys_login_log
			WHERE login_time >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
			GROUP BY HOUR(login_time)`)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row["hourValue"] != nil && row["total"] != nil {
				totals[framework.AsInt64(row["hourValue"])] = framework.AsInt64(row["total"])
			}
		}
		values := make([]int64, 0, 24)
		now := time.Now()
		for i := 23; i >= 0; i-- {
			values = append(values, totals[int64(now.Add(-time.Duration(i)*time.Hour).Hour())])
		}
		return values, nil
	}
	days := hours / 24
	// selectLoginTrendByDay
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT DATEDIFF(DATE(login_time), CURDATE() - INTERVAL (? - 1) DAY) AS dayIndex, COUNT(*) AS total
		FROM sys_login_log
		WHERE login_time >= CURDATE() - INTERVAL (? - 1) DAY
		GROUP BY dayIndex
		ORDER BY dayIndex`, days, days)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row["dayIndex"] != nil && row["total"] != nil {
			totals[framework.AsInt64(row["dayIndex"])] = framework.AsInt64(row["total"])
		}
	}
	values := make([]int64, 0, days)
	for i := 0; i < days; i++ {
		values = append(values, totals[int64(i)])
	}
	return values, nil
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
