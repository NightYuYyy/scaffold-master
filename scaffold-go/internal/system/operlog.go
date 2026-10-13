package system

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"scaffold/internal/framework"
)

// OperationLogRecorder 对应 Java JdbcOperationLogRecorder：同步写 sys_oper_log，独立于业务事务。
type OperationLogRecorder struct{ db *sql.DB }

func NewOperationLogRecorder(db *sql.DB) *OperationLogRecorder { return &OperationLogRecorder{db: db} }

func (r *OperationLogRecorder) Record(ctx context.Context, e framework.OperationLogEntry) {
	_, err := framework.Exec(context.WithoutCancel(ctx), r.db, `
		INSERT INTO sys_oper_log (module, action, request_method, request_url, request_params,
		    oper_name, dept_name, ip, location, cost, result, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Module, e.Action, e.RequestMethod, e.RequestURL, e.RequestParams,
		e.Operator, nil, e.IP, location(e.IP), e.Cost, e.Result, e.ErrorMessage)
	if err != nil {
		log.Printf("操作日志写入失败: %v", err)
	}
}

func location(ip string) *string {
	if strings.TrimSpace(ip) == "" {
		return nil
	}
	loc := "内网"
	if ip == "127.0.0.1" || ip == "0:0:0:0:0:0:0:1" || ip == "::1" {
		loc = "本机"
	}
	return &loc
}
