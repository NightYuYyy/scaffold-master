package app

// 对应 MonitorStatsApiTest。

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

var (
	statsModule    = "it_stats_" + uniqueSuffix()
	statsProbeIP   = fmt.Sprintf("10.250.%d.7", time.Now().UnixNano()%200)
	statsProbeUser = "probe_" + uniqueSuffix()
)

// statsInsertOper 对应 insertOper，并注册按 module 清理。
func statsInsertOper(t testing.TB, minutesAgo, result int, cost int64) {
	t.Helper()
	exec(t, `
                INSERT INTO sys_oper_log (module, action, oper_name, cost, result, oper_time)
                VALUES (?, '查询', 'admin', ?, ?, DATE_SUB(NOW(), INTERVAL ? MINUTE))
                `, statsModule, cost, result, minutesAgo)
	t.Cleanup(func() { exec(t, "DELETE FROM sys_oper_log WHERE module = ?", statsModule) })
}

func TestOperStatsReturnOverviewAndFullSeries(t *testing.T) {
	statsInsertOper(t, 0, 1, 40)
	statsInsertOper(t, 0, 0, 900)
	token := loginAsAdmin(t)

	day := okData(t, get(t, "/api/monitor/oper-logs/stats", token))
	if missing := hasKeys(day, "total", "failed", "successRate", "avgCost", "p95Cost", "activeUsers", "series"); len(missing) > 0 {
		t.Errorf("stats missing keys %v: %v", missing, day)
	}
	if num(day["failed"]) < 1 {
		t.Errorf("failed = %v, want >= 1", day["failed"])
	}
	if num(day["p95Cost"]) < 40 {
		t.Errorf("p95Cost = %v, want >= 40", day["p95Cost"])
	}
	series := asMaps(list(day, "series"))
	if len(series) != 24 {
		t.Errorf("series len = %d, want 24", len(series))
	}
	for i, row := range series {
		if missing := hasKeys(row, "bucket", "total", "failed"); len(missing) > 0 {
			t.Errorf("series[%d] missing %v: %v", i, missing, row)
		}
	}

	if l := list(okData(t, get(t, "/api/monitor/oper-logs/stats?range=7d", token)), "series"); len(l) != 7 {
		t.Errorf("7d series len = %d", len(l))
	}
	if l := list(okData(t, get(t, "/api/monitor/oper-logs/stats?range=30d", token)), "series"); len(l) != 30 {
		t.Errorf("30d series len = %d", len(l))
	}
	if d := okData(t, get(t, "/api/monitor/oper-logs/stats?range=unknown", token)); d["range"] != "24h" {
		t.Errorf("range = %v, want 24h", d["range"])
	}
}

func TestTimelineMergesCloseOperationsIntoBlocks(t *testing.T) {
	statsInsertOper(t, 0, 1, 10)
	statsInsertOper(t, 0, 0, 20)
	token := loginAsAdmin(t)

	timeline := okData(t, get(t, "/api/monitor/oper-logs/timeline", token))
	if timeline["nowSec"] == nil {
		t.Errorf("nowSec missing: %v", timeline)
	}
	var lane map[string]any
	for _, row := range asMaps(list(timeline, "lanes")) {
		if row["module"] == statsModule {
			lane = row
			break
		}
	}
	if lane == nil {
		t.Fatalf("no lane for module %s: %v", statsModule, timeline["lanes"])
	}
	blocks := asMaps(list(lane, "blocks"))
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1: %v", len(blocks), blocks)
	}
	if num(blocks[0]["count"]) != 2 {
		t.Errorf("count = %v, want 2", blocks[0]["count"])
	}
	if num(blocks[0]["failed"]) != 1 {
		t.Errorf("failed = %v, want 1", blocks[0]["failed"])
	}
	if num(blocks[0]["maxCost"]) != 20 {
		t.Errorf("maxCost = %v, want 20", blocks[0]["maxCost"])
	}
	if missing := hasKeys(blocks[0], "startSec", "endSec", "ids"); len(missing) > 0 {
		t.Errorf("block missing %v: %v", missing, blocks[0])
	}

	past := okData(t, get(t, "/api/monitor/oper-logs/timeline?date=2000-01-01", token))
	if past["nowSec"] != nil {
		t.Errorf("past nowSec = %v, want null", past["nowSec"])
	}
	if !emptyList(past["lanes"]) {
		t.Errorf("past lanes = %v, want []", past["lanes"])
	}
	if r := get(t, "/api/monitor/oper-logs/timeline?date=bad", token); r.Status != http.StatusBadRequest {
		t.Errorf("date=bad status %d: %s", r.Status, r.Raw)
	}
}

func TestLoginStatsReturnOverview(t *testing.T) {
	stats := okData(t, get(t, "/api/monitor/login-logs/stats?range=7d", loginAsAdmin(t)))
	if missing := hasKeys(stats, "total", "failed", "successRate", "users", "ips", "series"); len(missing) > 0 {
		t.Errorf("stats missing keys %v: %v", missing, stats)
	}
	if num(stats["total"]) < 1 {
		t.Errorf("total = %v, want >= 1", stats["total"])
	}
	if l := list(stats, "series"); len(l) != 7 {
		t.Errorf("series len = %d, want 7", len(l))
	}
}

func TestLoginHeatmapAggregatesCellsAndFailureAnomalies(t *testing.T) {
	t.Cleanup(func() { exec(t, "DELETE FROM sys_login_log WHERE ip = ?", statsProbeIP) })
	for i := 0; i < 6; i++ {
		exec(t, "INSERT INTO sys_login_log (username, ip, status, msg, login_time) VALUES (?, ?, 0, '密码错误', DATE_SUB(NOW(), INTERVAL ? MINUTE))",
			statsProbeUser, statsProbeIP, i)
	}
	heat := okData(t, get(t, "/api/monitor/login-logs/heatmap?days=7", loginAsAdmin(t)))
	if num(heat["days"]) != 7 {
		t.Errorf("days = %v, want 7", heat["days"])
	}
	if num(heat["threshold"]) < 1 {
		t.Errorf("threshold = %v, want >= 1", heat["threshold"])
	}
	cells := asMaps(list(heat, "cells"))
	if len(cells) == 0 {
		t.Errorf("cells empty: %v", heat)
	}
	for i, cell := range cells {
		if missing := hasKeys(cell, "dow", "hour", "total", "failed"); len(missing) > 0 {
			t.Errorf("cells[%d] missing %v: %v", i, missing, cell)
		}
	}
	anomalies := asMaps(list(heat, "anomalies"))
	ipHit, userHit := false, false
	for _, a := range anomalies {
		if a["kind"] == "ip" && a["key"] == statsProbeIP && num(a["failed"]) >= 6 {
			ipHit = true
		}
		if a["kind"] == "user" && a["key"] == statsProbeUser {
			userHit = true
		}
	}
	if !ipHit {
		t.Errorf("no ip anomaly for %s with failed>=6: %v", statsProbeIP, anomalies)
	}
	if !userHit {
		t.Errorf("no user anomaly for %s: %v", statsProbeUser, anomalies)
	}
	if d := okData(t, get(t, "/api/monitor/login-logs/heatmap?days=99", loginAsAdmin(t))); num(d["days"]) != 30 {
		t.Errorf("days=99 → days = %v, want 30", d["days"])
	}
}

func TestStatsRequireLogPermissions(t *testing.T) {
	username, _ := readOnlyUserFixture(t, "stats_read_")
	token := login(t, username, "admin123")
	for _, path := range []string{"/api/monitor/oper-logs/stats", "/api/monitor/oper-logs/timeline", "/api/monitor/login-logs/stats", "/api/monitor/login-logs/heatmap"} {
		if r := get(t, path, token); r.Status != http.StatusForbidden {
			t.Errorf("%s status %d: %s", path, r.Status, r.Raw)
		}
		if r := get(t, path, ""); r.Status != http.StatusUnauthorized {
			t.Errorf("anonymous %s status %d: %s", path, r.Status, r.Raw)
		}
	}
}
