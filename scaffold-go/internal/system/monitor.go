package system

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"scaffold/internal/framework"
)

// MonitorStatsService 对应 Java MonitorStatsService + MonitorStatsMapper：
// 操作 / 登录日志概览与趋势、登录热力与异常聚合、当日按模块聚合的时间轴泳道。
type MonitorStatsService struct{ db *sql.DB }

func NewMonitorStatsService(db *sql.DB) *MonitorStatsService { return &MonitorStatsService{db: db} }

const (
	monMergeGapSeconds       = 15 * 60 // 同一模块两次操作间隔不超过 15 分钟时合并为一个时间块
	monBlockIDLimit          = 50      // 每个时间块最多回传的日志 ID 数
	monDefaultAlertThreshold = 5       // 登录失败达到该次数即列为异常
	monDBTime                = "2006-01-02 15:04:05"
)

type monWindow struct {
	rng     string
	hourly  bool
	buckets int
	nowTime time.Time
	now     string
	since   string
}

// selectDbNow：时间边界统一由数据库时钟给出。
func (s *MonitorStatsService) dbNow(ctx context.Context) (time.Time, error) {
	row, err := framework.SelectRow(ctx, s.db, `SELECT DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s') AS now`)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(monDBTime, framework.StrOrEmpty(row["now"]))
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (s *MonitorStatsService) window(ctx context.Context, rng string) (monWindow, error) {
	now, err := s.dbNow(ctx)
	if err != nil {
		return monWindow{}, err
	}
	w := monWindow{nowTime: now, now: now.Format(monDBTime)}
	switch rng {
	case "7d":
		w.rng, w.hourly, w.buckets = "7d", false, 7
		w.since = startOfDay(now).AddDate(0, 0, -6).Format(monDBTime)
	case "30d":
		w.rng, w.hourly, w.buckets = "30d", false, 30
		w.since = startOfDay(now).AddDate(0, 0, -29).Format(monDBTime)
	default:
		w.rng, w.hourly, w.buckets = "24h", true, 24
		w.since = now.Add(-24 * time.Hour).Format(monDBTime)
	}
	return w, nil
}

func monSuccessRate(total, failed int64) float64 {
	if total > 0 {
		return javaRound(float64(total-failed)*1000.0/float64(total)) / 10.0
	}
	return 100.0
}

// number 对应 Java number(Object)：非数值 → 0。
func monNumber(v any) int64 { return framework.AsInt64(v) }

// OperStats 对应 operStats。
func (s *MonitorStatsService) OperStats(ctx context.Context, rng string) (map[string]any, error) {
	w, err := s.window(ctx, rng)
	if err != nil {
		return nil, err
	}
	// selectOperOverview
	overview, err := framework.SelectRow(ctx, s.db, `
		SELECT COUNT(*) AS total,
		       COALESCE(SUM(result = 0), 0) AS failed,
		       COALESCE(ROUND(AVG(cost)), 0) AS avgCost,
		       COUNT(cost) AS costCount,
		       COUNT(DISTINCT oper_name) AS activeUsers
		FROM sys_oper_log
		WHERE oper_time >= ?`, w.since)
	if err != nil {
		return nil, err
	}
	total, failed, costCount := monNumber(overview["total"]), monNumber(overview["failed"]), monNumber(overview["costCount"])
	var p95 int64
	if costCount > 0 {
		// selectOperCostAt
		p95, err = framework.SelectInt64(ctx, s.db, `
			SELECT cost
			FROM sys_oper_log
			WHERE oper_time >= ? AND cost IS NOT NULL
			ORDER BY cost
			LIMIT 1 OFFSET ?`, w.since, int64(math.Ceil(float64(costCount)*0.95))-1)
		if err != nil {
			return nil, err
		}
	}
	var rows []framework.Row
	if w.hourly {
		// selectOperHourly
		rows, err = framework.SelectRows(ctx, s.db, `
			SELECT TIMESTAMPDIFF(HOUR, oper_time, ?) AS ago, COUNT(*) AS total, COALESCE(SUM(result = 0), 0) AS failed
			FROM sys_oper_log
			WHERE oper_time >= ?
			GROUP BY ago`, w.now, w.since)
	} else {
		// selectOperDaily
		rows, err = framework.SelectRows(ctx, s.db, `
			SELECT DATEDIFF(DATE(?), DATE(oper_time)) AS ago, COUNT(*) AS total, COALESCE(SUM(result = 0), 0) AS failed
			FROM sys_oper_log
			WHERE oper_time >= ?
			GROUP BY ago`, w.now, w.since)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"range":       w.rng,
		"total":       total,
		"failed":      failed,
		"successRate": monSuccessRate(total, failed),
		"avgCost":     monNumber(overview["avgCost"]),
		"p95Cost":     p95,
		"activeUsers": monNumber(overview["activeUsers"]),
		"series":      monSeries(w, rows),
	}, nil
}

// LoginStats 对应 loginStats。
func (s *MonitorStatsService) LoginStats(ctx context.Context, rng string) (map[string]any, error) {
	w, err := s.window(ctx, rng)
	if err != nil {
		return nil, err
	}
	// selectLoginOverview
	overview, err := framework.SelectRow(ctx, s.db, `
		SELECT COUNT(*) AS total,
		       COALESCE(SUM(status = 0), 0) AS failed,
		       COUNT(DISTINCT username) AS users,
		       COUNT(DISTINCT ip) AS ips
		FROM sys_login_log
		WHERE login_time >= ?`, w.since)
	if err != nil {
		return nil, err
	}
	total, failed := monNumber(overview["total"]), monNumber(overview["failed"])
	var rows []framework.Row
	if w.hourly {
		// selectLoginHourly
		rows, err = framework.SelectRows(ctx, s.db, `
			SELECT TIMESTAMPDIFF(HOUR, login_time, ?) AS ago, COUNT(*) AS total, COALESCE(SUM(status = 0), 0) AS failed
			FROM sys_login_log
			WHERE login_time >= ?
			GROUP BY ago`, w.now, w.since)
	} else {
		// selectLoginDaily
		rows, err = framework.SelectRows(ctx, s.db, `
			SELECT DATEDIFF(DATE(?), DATE(login_time)) AS ago, COUNT(*) AS total, COALESCE(SUM(status = 0), 0) AS failed
			FROM sys_login_log
			WHERE login_time >= ?
			GROUP BY ago`, w.now, w.since)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"range":       w.rng,
		"total":       total,
		"failed":      failed,
		"successRate": monSuccessRate(total, failed),
		"users":       monNumber(overview["users"]),
		"ips":         monNumber(overview["ips"]),
		"series":      monSeries(w, rows),
	}, nil
}

// LoginHeatmap 对应 loginHeatmap：最近 days 天按「星期 × 小时」聚合 + 同 IP / 同账号失败聚合告警。
func (s *MonitorStatsService) LoginHeatmap(ctx context.Context, days int) (map[string]any, error) {
	span := 7
	if days >= 30 {
		span = 30
	}
	now, err := s.dbNow(ctx)
	if err != nil {
		return nil, err
	}
	since := startOfDay(now).AddDate(0, 0, -(span - 1)).Format(monDBTime)
	threshold := monDefaultAlertThreshold
	// selectLoginAlertThreshold
	if row, err := framework.SelectRow(ctx, s.db, `SELECT config_value FROM sys_config WHERE config_key = 'sys.login.alertThreshold' LIMIT 1`); err != nil {
		return nil, err
	} else if row != nil && row["config_value"] != nil {
		if configured := strings.TrimSpace(framework.StrOrEmpty(row["config_value"])); configured != "" {
			// 参数值不是数字时沿用默认阈值
			if n, err := strconv.Atoi(configured); err == nil {
				threshold = max(1, n)
			}
		}
	}
	// selectLoginHeatmap：星期（0 周一 … 6 周日）× 小时
	heat, err := framework.SelectRows(ctx, s.db, `
		SELECT WEEKDAY(login_time) AS dow, HOUR(login_time) AS hour,
		       COUNT(*) AS total, COALESCE(SUM(status = 0), 0) AS failed
		FROM sys_login_log
		WHERE login_time >= ?
		GROUP BY dow, hour`, since)
	if err != nil {
		return nil, err
	}
	cells := make([]map[string]any, 0, len(heat))
	for _, row := range heat {
		cells = append(cells, map[string]any{
			"dow": monNumber(row["dow"]), "hour": monNumber(row["hour"]),
			"total": monNumber(row["total"]), "failed": monNumber(row["failed"]),
		})
	}
	// selectLoginIpAnomalies：同一 IP 失败次数达到阈值，users 为该 IP 尝试过的不同账号数
	ipRows, err := framework.SelectRows(ctx, s.db, `
		SELECT ip AS `+"`key`"+`,
		       COALESCE(SUM(status = 0), 0) AS failed,
		       COUNT(*) AS total,
		       COUNT(DISTINCT username) AS users,
		       DATE_FORMAT(MIN(login_time), '%Y-%m-%d %H:%i') AS firstTime,
		       DATE_FORMAT(MAX(login_time), '%Y-%m-%d %H:%i') AS lastTime
		FROM sys_login_log
		WHERE login_time >= ? AND ip IS NOT NULL AND ip <> ''
		GROUP BY ip
		HAVING failed >= ?
		ORDER BY failed DESC, lastTime DESC
		LIMIT 12`, since, threshold)
	if err != nil {
		return nil, err
	}
	// selectLoginUserAnomalies：同一账号失败次数达到阈值，users 位置返回来源 IP 数
	userRows, err := framework.SelectRows(ctx, s.db, `
		SELECT username AS `+"`key`"+`,
		       COALESCE(SUM(status = 0), 0) AS failed,
		       COUNT(*) AS total,
		       COUNT(DISTINCT ip) AS users,
		       DATE_FORMAT(MIN(login_time), '%Y-%m-%d %H:%i') AS firstTime,
		       DATE_FORMAT(MAX(login_time), '%Y-%m-%d %H:%i') AS lastTime
		FROM sys_login_log
		WHERE login_time >= ? AND username IS NOT NULL AND username <> ''
		GROUP BY username
		HAVING failed >= ?
		ORDER BY failed DESC, lastTime DESC
		LIMIT 12`, since, threshold)
	if err != nil {
		return nil, err
	}
	anomalies := make([]map[string]any, 0, len(ipRows)+len(userRows))
	for _, row := range ipRows {
		anomalies = append(anomalies, monAnomaly("ip", row))
	}
	for _, row := range userRows {
		anomalies = append(anomalies, monAnomaly("user", row))
	}
	sort.SliceStable(anomalies, func(i, j int) bool {
		return anomalies[i]["failed"].(int64) > anomalies[j]["failed"].(int64)
	})
	return map[string]any{
		"days":      span,
		"since":     since,
		"threshold": threshold,
		"cells":     cells,
		"anomalies": anomalies,
	}, nil
}

func monAnomaly(kind string, row framework.Row) map[string]any {
	return map[string]any{
		"kind":      kind,
		"key":       framework.AsString(row["key"]),
		"failed":    monNumber(row["failed"]),
		"total":     monNumber(row["total"]),
		"spread":    monNumber(row["users"]),
		"firstTime": row["firstTime"],
		"lastTime":  row["lastTime"],
	}
}

// OperTimeline 对应 operTimeline：按模块分道，同模块相邻操作间隔不超过 15 分钟合为一块；
// 时间以当日秒数表示（0–86400）。date 为 nil 时取数据库当天。
func (s *MonitorStatsService) OperTimeline(ctx context.Context, date *time.Time) (map[string]any, error) {
	dbNow, err := s.dbNow(ctx)
	if err != nil {
		return nil, err
	}
	day := startOfDay(dbNow)
	if date != nil {
		day = startOfDay(*date)
	}
	// selectOperPointsOfDay
	points, err := framework.SelectRows(ctx, s.db, `
		SELECT id,
		       module,
		       TIME_TO_SEC(TIME(oper_time)) AS sec,
		       result,
		       cost
		FROM sys_oper_log
		WHERE oper_time >= ? AND oper_time < DATE_ADD(?, INTERVAL 1 DAY)
		ORDER BY module, oper_time, id
		LIMIT 5000`, day.Format(monDBTime), day.Format(monDBTime))
	if err != nil {
		return nil, err
	}
	laneOrder := []string{}
	lanes := map[string][]map[string]any{}
	open := map[string]map[string]any{}
	for _, point := range points {
		module := framework.AsString(point["module"])
		sec := monNumber(point["sec"])
		block := open[module]
		if block == nil || sec-block["endSec"].(int64) > monMergeGapSeconds {
			block = map[string]any{"startSec": sec, "endSec": sec, "count": int64(0), "failed": int64(0), "maxCost": int64(0), "ids": []int64{}}
			if _, seen := lanes[module]; !seen {
				laneOrder = append(laneOrder, module)
			}
			lanes[module] = append(lanes[module], block)
			open[module] = block
		}
		// absorb
		block["endSec"] = sec
		block["count"] = block["count"].(int64) + 1
		if monNumber(point["result"]) == 0 {
			block["failed"] = block["failed"].(int64) + 1
		}
		block["maxCost"] = max(block["maxCost"].(int64), monNumber(point["cost"]))
		if ids := block["ids"].([]int64); len(ids) < monBlockIDLimit {
			block["ids"] = append(ids, monNumber(point["id"]))
		}
	}
	laneRows := make([]map[string]any, 0, len(laneOrder))
	for _, module := range laneOrder {
		blocks := lanes[module]
		var count int64
		for _, b := range blocks {
			count += b["count"].(int64)
		}
		laneRows = append(laneRows, map[string]any{"module": module, "count": count, "blocks": blocks})
	}
	sort.SliceStable(laneRows, func(i, j int) bool { return laneRows[i]["count"].(int64) > laneRows[j]["count"].(int64) })
	var nowSec any
	if day.Equal(startOfDay(dbNow)) {
		nowSec = dbNow.Hour()*3600 + dbNow.Minute()*60 + dbNow.Second()
	}
	return map[string]any{
		"date":   day.Format("2006-01-02"),
		"nowSec": nowSec,
		"lanes":  laneRows,
	}, nil
}

func monSeries(w monWindow, rows []framework.Row) []map[string]any {
	byAgo := map[int64]framework.Row{}
	for _, row := range rows {
		byAgo[monNumber(row["ago"])] = row
	}
	result := make([]map[string]any, 0, w.buckets)
	for i := 0; i < w.buckets; i++ {
		ago := w.buckets - 1 - i
		var label string
		if w.hourly {
			label = w.nowTime.Add(-time.Duration(ago) * time.Hour).Format("15:00")
		} else {
			label = startOfDay(w.nowTime).AddDate(0, 0, -ago).Format("01-02")
		}
		bucket := map[string]any{"bucket": label, "total": int64(0), "failed": int64(0)}
		if row := byAgo[int64(ago)]; row != nil {
			bucket["total"] = monNumber(row["total"])
			bucket["failed"] = monNumber(row["failed"])
		}
		result = append(result, bucket)
	}
	return result
}

// RegisterMonitorStatsRoutes 对应 MonitorController 中的 4 个统计接口（日志列表/导出/删除在 management.go）。
func RegisterMonitorStatsRoutes(mux *http.ServeMux, d Deps, s *MonitorStatsService) {
	mux.Handle("GET /api/monitor/oper-logs/stats", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		data, err := s.OperStats(r.Context(), framework.QueryStringDefault(r, "range", "24h"))
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}, d.Security.Perm("monitor:operlog:list")))

	mux.Handle("GET /api/monitor/oper-logs/timeline", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var date *time.Time
		if v := framework.QueryString(r, "date"); v != nil && *v != "" {
			t, err := time.Parse("2006-01-02", *v)
			if err != nil {
				return framework.TypeMismatch("date")
			}
			date = &t
		}
		data, err := s.OperTimeline(r.Context(), date)
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}, d.Security.Perm("monitor:operlog:list")))

	mux.Handle("GET /api/monitor/login-logs/stats", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		data, err := s.LoginStats(r.Context(), framework.QueryStringDefault(r, "range", "24h"))
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}, d.Security.Perm("monitor:loginlog:list")))

	mux.Handle("GET /api/monitor/login-logs/heatmap", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		days, err := framework.QueryIntDefault(r, "days", 7)
		if err != nil {
			return err
		}
		data, err := s.LoginHeatmap(r.Context(), days)
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}, d.Security.Perm("monitor:loginlog:list")))
}
