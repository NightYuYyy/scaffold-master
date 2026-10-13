package platform

import (
	"context"
	"net/http"
	"strings"

	"scaffold/internal/framework"
)

// ---------- 账户流水 ----------

func (s *svc) adminCounterLogFilter(r *http.Request) (counterLogFilter, error) {
	appID, err := s.resolveAppID(r)
	if err != nil {
		return counterLogFilter{}, err
	}
	userID, err := framework.QueryInt64(r, "userId")
	if err != nil {
		return counterLogFilter{}, err
	}
	return counterLogFilter{appID: appID, userID: userID, fieldKey: framework.QueryString(r, "fieldKey"), keyword: framework.QueryString(r, "keyword")}, nil
}

func (s *svc) adminCounterLogs(r *http.Request) (any, error) {
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	f, err := s.adminCounterLogFilter(r)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.counterLogs(r.Context(), f, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *svc) adminExportCounterLogs(r *http.Request) (string, error) {
	f, err := s.adminCounterLogFilter(r)
	if err != nil {
		return "", err
	}
	rows, _, err := s.counterLogs(r.Context(), f, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["username"], row["fieldKey"], row["amount"], row["balanceAfter"], row["reason"], row["refNo"], row["operator"], row["createTime"]})
	}
	return csvOf([]string{"账号", "字段", "变动量", "变动后余额", "原因", "幂等号", "操作方", "时间"}, data), nil
}

// ---------- 认证日志 ----------

type authLogFilter struct {
	appID   int64
	event   *string
	status  *int
	keyword *string
}

func (s *svc) authLogs(ctx context.Context, f authLogFilter, offset, limit int) ([]framework.Row, int64, error) {
	where := " WHERE app_id = ?"
	args := []any{f.appID}
	if f.event != nil && strings.TrimSpace(*f.event) != "" {
		where += " AND event = ?"
		args = append(args, strings.TrimSpace(*f.event))
	}
	if f.status != nil {
		where += " AND status = ?"
		args = append(args, *f.status)
	}
	if kw := like(f.keyword); kw != "" {
		where += " AND (username LIKE ? OR ip LIKE ?)"
		args = append(args, kw, kw)
	}
	total, err := framework.SelectInt64(ctx, s.db, "SELECT COUNT(*) FROM app_auth_log"+where, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := framework.SelectRows(ctx, s.db, `
		SELECT id, app_id AS appId, user_id AS userId, username, event, status, ip, msg,
		       DATE_FORMAT(event_time, '%Y-%m-%d %H:%i:%s') AS eventTime
		FROM app_auth_log`+where+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	return rows, total, err
}

func (s *svc) adminAuthLogFilter(r *http.Request) (authLogFilter, error) {
	appID, err := s.resolveAppID(r)
	if err != nil {
		return authLogFilter{}, err
	}
	status, err := framework.QueryInt(r, "status")
	if err != nil {
		return authLogFilter{}, err
	}
	return authLogFilter{appID: appID, event: framework.QueryString(r, "event"), status: status, keyword: framework.QueryString(r, "keyword")}, nil
}

func (s *svc) adminAuthLogs(r *http.Request) (any, error) {
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	f, err := s.adminAuthLogFilter(r)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.authLogs(r.Context(), f, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *svc) adminExportAuthLogs(r *http.Request) (string, error) {
	f, err := s.adminAuthLogFilter(r)
	if err != nil {
		return "", err
	}
	rows, _, err := s.authLogs(r.Context(), f, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		result := "失败"
		if framework.AsInt64(row["status"]) == 1 {
			result = "成功"
		}
		data = append(data, []any{row["username"], row["event"], result, row["ip"], row["msg"], row["eventTime"]})
	}
	return csvOf([]string{"账号", "事件", "结果", "IP", "说明", "时间"}, data), nil
}

// ---------- CSV ----------

// csvOf 与 system 包的导出约定一致：中文表头、\n 换行、仅在需要时加引号。
func csvOf(headers []string, rows [][]any) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(headers, ","))
	sb.WriteByte('\n')
	for _, row := range rows {
		for i, v := range row {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(csvCell(v))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func csvCell(v any) string {
	text := framework.StrOrEmpty(v)
	if strings.Contains(text, `"`) {
		text = strings.ReplaceAll(text, `"`, `""`)
	}
	if strings.ContainsAny(text, ",\"\n\r") {
		return `"` + text + `"`
	}
	return text
}
