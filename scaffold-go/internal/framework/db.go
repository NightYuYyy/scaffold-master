package framework

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DBConfig 对应 application-dev.yml 的数据源配置。
type DBConfig struct {
	Host, Port, Name, Username, Password, TimeZone string
}

// DSN 生成 go-sql-driver DSN：像 Hikari forceConnectionTimeZoneToSession 一样设置会话时区；
// clientFoundRows=true 让 UPDATE 的影响行数与 JDBC 一致（返回匹配行数而非变更行数），
// 业务里大量 `update == 0 → 不存在` 的判断依赖这一点。
func (c DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&clientFoundRows=true&time_zone=%s",
		c.Username, c.Password, c.Host, c.Port, c.Name, url.QueryEscape("'"+c.TimeZone+"'"))
}

// OpenDB 打开连接池并 ping。
func OpenDB(c DBConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", c.DSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return db, db.PingContext(ctx)
}

// Querier 同时被 *sql.DB 与 *sql.Tx 实现。
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Row 对应 MyBatis 返回的 Map<String,Object>：key 为列标签原样，值类型见 convertValue。
type Row = map[string]any

// SelectRows 执行查询并把每行转为 Row；空结果返回空切片（非 nil）。
func SelectRows(ctx context.Context, q Querier, query string, args ...any) ([]Row, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w\nsql: %s", err, query)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(Row, len(cols))
		for i, c := range cols {
			row[c] = convertValue(types[i], vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SelectRow 返回首行；无结果返回 nil（对应 MyBatis 单行 Map 返回 null）。
func SelectRow(ctx context.Context, q Querier, query string, args ...any) (Row, error) {
	rows, err := SelectRows(ctx, q, query, args...)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

// SelectInt64 读取单值（COUNT 等）；无结果返回 0。
func SelectInt64(ctx context.Context, q Querier, query string, args ...any) (int64, error) {
	row, err := SelectRow(ctx, q, query, args...)
	if err != nil || row == nil {
		return 0, err
	}
	for _, v := range row {
		return AsInt64(v), nil
	}
	return 0, nil
}

// SelectStrings 读取单列字符串列表（NULL 跳过）。
func SelectStrings(ctx context.Context, q Querier, query string, args ...any) ([]string, error) {
	rows, err := SelectRows(ctx, q, query, args...)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		for _, v := range r {
			if v != nil {
				out = append(out, fmt.Sprint(v))
			}
		}
	}
	return out, nil
}

// SelectInt64s 读取单列整数列表。
func SelectInt64s(ctx context.Context, q Querier, query string, args ...any) ([]int64, error) {
	rows, err := SelectRows(ctx, q, query, args...)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		for _, v := range r {
			if v != nil {
				out = append(out, AsInt64(v))
			}
		}
	}
	return out, nil
}

// Exec 执行写语句，返回影响行数。
func Exec(ctx context.Context, q Querier, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("exec: %w\nsql: %s", err, query)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Insert 执行 INSERT 并返回自增主键（对应 @Options(useGeneratedKeys)）。
func Insert(ctx context.Context, q Querier, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("insert: %w\nsql: %s", err, query)
	}
	return res.LastInsertId()
}

// Tx 对应 @Transactional：fn 返回 error 或 panic 时回滚。
func Tx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()
	return fn(tx)
}

// In 生成 n 个占位符 "?, ?, ?"（对应 MyBatis foreach）。
func In(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Int64Args 把 []int64 展开为 []any。
func Int64Args(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// convertValue 把驱动值转换为与 MyBatis + Jackson 一致的 JSON 友好类型：
// 整数列 → int64，DECIMAL → json.Number（保留小数位），浮点 → float64，
// DATETIME/TIMESTAMP → "2006-01-02T15:04:05"，DATE → "2006-01-02"，文本 → string。
func convertValue(ct *sql.ColumnType, v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case int64, uint64, int32, int8, int16, uint8, uint16, uint32, int, float32, float64, bool, string:
		return t
	case time.Time:
		if ct.DatabaseTypeName() == "DATE" {
			return t.Format("2006-01-02")
		}
		return t.Format("2006-01-02T15:04:05")
	case []byte:
		s := string(t)
		switch strings.TrimPrefix(ct.DatabaseTypeName(), "UNSIGNED ") {
		case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "BIGINT", "YEAR":
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
			return s
		case "DECIMAL", "FLOAT", "DOUBLE":
			return json.Number(s)
		}
		return s
	}
	return v
}

// ---------- Row 取值辅助（对应 Java 中 (Number) row.get(k) 等转换） ----------

// AsInt64 把任意数值类型转为 int64；nil/非数值返回 0。
func AsInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case uint64:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		f, _ := t.Float64()
		return int64(f) // BigDecimal.longValue() 截断
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(t), 10, 64)
		return n
	}
	return 0
}

// AsInt64Ptr 对应 numberAsLong：nil → nil。
func AsInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	n := AsInt64(v)
	return &n
}

// AsFloat64 把任意数值类型转为 float64。
func AsFloat64(v any) float64 {
	switch t := v.(type) {
	case int64:
		return float64(t)
	case int:
		return float64(t)
	case float64:
		return t
	case float32:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// AsString 对应 String.valueOf(v)：nil → "null"（与 Java 行为一致，调用方自行判空）。
func AsString(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprint(v)
}

// StrOrEmpty 对应 value == null ? "" : String.valueOf(value)。
func StrOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
