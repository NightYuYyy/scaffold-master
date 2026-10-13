package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"scaffold/internal/framework"
)

// 余额与变动量用 1e4 定点整数（big.Int）运算，与 DECIMAL(20,4) 精确对应。

var (
	scale4   = big.NewInt(10000)
	numberRe = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][-+]?\d+)?$`)
)

// parseDec4 解析十进制文本为定点整数；超过 4 位小数或格式非法返回 false。
func parseDec4(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if !numberRe.MatchString(s) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, false
	}
	r.Mul(r, new(big.Rat).SetInt(scale4))
	if !r.IsInt() {
		return nil, false
	}
	return new(big.Int).Set(r.Num()), true
}

// formatDec4 定点整数 → 去掉多余 0 的十进制文本（作为 JSON 数字输出）。
func formatDec4(n *big.Int) json.Number {
	abs := new(big.Int).Abs(n)
	q, rem := new(big.Int).QuoRem(abs, scale4, new(big.Int))
	s := q.String()
	if rem.Sign() != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%04d", rem.Int64()), "0")
	}
	if n.Sign() < 0 {
		s = "-" + s
	}
	return json.Number(s)
}

// trimDec 把 DECIMAL 列值（如 "100.0000"）规整为 JSON 数字 "100"。
func trimDec(v any) json.Number {
	if n, ok := parseDec4(framework.StrOrEmpty(v)); ok {
		return formatDec4(n)
	}
	return json.Number("0")
}

// parseAmount 变动量：非 0 数字，最多 4 位小数；接受 JSON 数字或数字字符串。
func parseAmount(raw json.RawMessage) (*big.Int, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, framework.Invalid("amount", "不能为空")
	}
	if strings.HasPrefix(s, `"`) {
		_ = json.Unmarshal(raw, &s)
	}
	n, ok := parseDec4(s)
	if !ok || n.Sign() == 0 {
		return nil, framework.Invalid("amount", "必须为非 0 数字且最多 4 位小数")
	}
	return n, nil
}

// counterResult 账户加减结果。
type counterResult struct {
	Balance    json.Number `json:"balance"`
	LogID      int64       `json:"logId"`
	Duplicated bool        `json:"duplicated"`
}

// changeCounter 事务内锁账户行后加减；refNo 幂等；按字段 options 校验负余额与上限。
func (s *svc) changeCounter(ctx context.Context, field *fieldDef, userID int64, amount *big.Int, reason, refNo, operator string) (*counterResult, error) {
	allowNegative, max := field.counterOptions()
	var res counterResult
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		// 1. 创建或排他锁定账户行，避免空行 FOR UPDATE 的间隙锁互相阻塞插入。
		if _, err := framework.Exec(ctx, tx, `INSERT INTO app_counter (app_id, user_id, field_key, balance) VALUES (?, ?, ?, 0) ON DUPLICATE KEY UPDATE id = id`, field.AppID, userID, field.Key); err != nil {
			return err
		}
		const lock = `SELECT id, balance FROM app_counter WHERE app_id = ? AND user_id = ? AND field_key = ? FOR UPDATE`
		row, err := framework.SelectRow(ctx, tx, lock, field.AppID, userID, field.Key)
		if err != nil {
			return err
		}
		// 2. 幂等：同一 refNo 且同账户同金额 → 返回首次结果；否则冲突
		dup, err := framework.SelectRow(ctx, tx, `SELECT id, user_id, field_key, amount, balance_after FROM app_counter_log WHERE app_id = ? AND ref_no = ?`, field.AppID, refNo)
		if err != nil {
			return err
		}
		if dup != nil {
			prev, _ := parseDec4(framework.StrOrEmpty(dup["amount"]))
			if framework.AsInt64(dup["user_id"]) == userID && framework.StrOrEmpty(dup["field_key"]) == field.Key && prev != nil && prev.Cmp(amount) == 0 {
				res = counterResult{Balance: trimDec(dup["balance_after"]), LogID: framework.AsInt64(dup["id"]), Duplicated: true}
				return nil
			}
			return framework.Biz("幂等号已使用")
		}
		// 3. 计算并校验
		balance, _ := parseDec4(framework.StrOrEmpty(row["balance"]))
		if balance == nil {
			balance = new(big.Int)
		}
		next := new(big.Int).Add(balance, amount)
		if next.Sign() < 0 && !allowNegative {
			return framework.Biz("余额不足")
		}
		if max != nil && next.Cmp(max) > 0 {
			return framework.Biz("超出账户上限")
		}
		nextText := formatDec4(next).String()
		if _, err := framework.Exec(ctx, tx, `UPDATE app_counter SET balance = ? WHERE id = ?`, nextText, framework.AsInt64(row["id"])); err != nil {
			return err
		}
		logID, err := framework.Insert(ctx, tx, `
			INSERT INTO app_counter_log (app_id, user_id, field_key, amount, balance_after, reason, ref_no, operator)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			field.AppID, userID, field.Key, formatDec4(amount).String(), nextText, reason, refNo, operator)
		if isDuplicate(err) {
			return framework.Biz("幂等号已使用")
		}
		if err != nil {
			return err
		}
		res = counterResult{Balance: json.Number(nextText), LogID: logID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// counterLogFilter 流水查询条件。
type counterLogFilter struct {
	appID    int64
	userID   *int64
	fieldKey *string
	keyword  *string
}

const counterLogSelect = `
	SELECT l.id, l.app_id AS appId, l.user_id AS userId, u.username, l.field_key AS fieldKey, l.amount,
	       l.balance_after AS balanceAfter, l.reason, l.ref_no AS refNo, l.operator,
	       DATE_FORMAT(l.create_time, '%Y-%m-%d %H:%i:%s') AS createTime
	FROM app_counter_log l
	LEFT JOIN app_user u ON u.id = l.user_id`

// counterLogs 分页查流水（id 倒序），金额规整为 JSON 数字。
func (s *svc) counterLogs(ctx context.Context, f counterLogFilter, offset, limit int) ([]framework.Row, int64, error) {
	where := " WHERE l.app_id = ?"
	args := []any{f.appID}
	if f.userID != nil {
		where += " AND l.user_id = ?"
		args = append(args, *f.userID)
	}
	if f.fieldKey != nil && *f.fieldKey != "" {
		where += " AND l.field_key = ?"
		args = append(args, *f.fieldKey)
	}
	if kw := like(f.keyword); kw != "" {
		where += " AND (l.reason LIKE ? OR l.ref_no LIKE ? OR u.username LIKE ?)"
		args = append(args, kw, kw, kw)
	}
	total, err := framework.SelectInt64(ctx, s.db, "SELECT COUNT(*) FROM app_counter_log l LEFT JOIN app_user u ON u.id = l.user_id"+where, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := framework.SelectRows(ctx, s.db, counterLogSelect+where+" ORDER BY l.id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	for _, r := range rows {
		r["amount"] = trimDec(r["amount"])
		r["balanceAfter"] = trimDec(r["balanceAfter"])
	}
	return rows, total, nil
}
