// Package generator 对应 Java 的 scaffold-generator 模块：代码生成器（导入表、配置、预览、下载）。
package generator

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"scaffold/internal/framework"
)

// Service 对应 Java GeneratorService。
type Service struct{ db *sql.DB }

func (s *Service) tables(ctx context.Context, q framework.PageQuery, keyword *string) (framework.PageResult[framework.Row], error) {
	rows, err := selectTables(ctx, s.db, keyword, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return framework.PageResult[framework.Row]{}, err
	}
	total, err := countTables(ctx, s.db, keyword)
	if err != nil {
		return framework.PageResult[framework.Row]{}, err
	}
	return framework.PageOf(booleanRows(rows, "synced"), total, q), nil
}

func (s *Service) columns(ctx context.Context, tableID int64) ([]framework.Row, error) {
	if _, err := s.ensureTable(ctx, tableID); err != nil {
		return nil, err
	}
	rows, err := selectColumns(ctx, s.db, tableID)
	return booleanRows(rows, "insert", "edit", "list", "query", "required"), err
}

func (s *Service) importableTables(ctx context.Context, keyword *string) ([]framework.Row, error) {
	rows, err := selectImportableTables(ctx, s.db, keyword)
	return booleanRows(rows, "imported"), err
}

// importTables：@Transactional。tableNames 为请求体中的 []any（Java 未检查的 List<String>）。
func (s *Service) importTables(ctx context.Context, tableNames []any) (map[string]any, error) {
	if len(tableNames) == 0 {
		return nil, framework.Biz("请选择要导入的数据库表")
	}
	ids := []int64{}
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		for _, tableName := range tableNames {
			if _, ok := tableName.(string); !ok && tableName != nil {
				return errors.New("tableNames 元素不是字符串") // Java: ClassCastException → 500
			}
			existingID, err := selectTableIdByName(ctx, tx, tableName)
			if err != nil {
				return err
			}
			if existingID != nil {
				ids = append(ids, *existingID)
				continue
			}
			dbTable, err := selectDatabaseTable(ctx, tx, tableName)
			if err != nil {
				return err
			}
			if dbTable == nil {
				return framework.Biz("数据库表不存在：" + framework.AsString(tableName))
			}
			tableID, err := insertTable(ctx, tx, tableRow(dbTable), currentUsername(ctx))
			if err != nil {
				return err
			}
			dbColumns, err := selectDatabaseColumns(ctx, tx, tableName)
			if err != nil {
				return err
			}
			for _, column := range dbColumns {
				if err := insertColumn(ctx, tx, columnRow(tableID, column)); err != nil {
					return err
				}
			}
			ids = append(ids, tableID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"imported": len(ids), "ids": ids}, nil
}

type previewResponse struct {
	Files      []genFile `json:"files"`
	ActiveFile string    `json:"activeFile"`
	Code       string    `json:"code"`
}

func (s *Service) preview(ctx context.Context, tableID int64) (*previewResponse, error) {
	table, err := s.ensureTable(ctx, tableID)
	if err != nil {
		return nil, err
	}
	columns, err := selectColumns(ctx, s.db, tableID)
	if err != nil {
		return nil, err
	}
	resp := &previewResponse{
		Files:      renderFiles(table, columns),
		ActiveFile: activeFileName(framework.AsString(table["tableName"])),
	}
	for _, f := range resp.Files {
		if f.Name == resp.ActiveFile {
			resp.Code = f.Code
		}
	}
	return resp, nil
}

// updateTable：@Transactional。
func (s *Service) updateTable(ctx context.Context, id int64, request map[string]any) error {
	var tableComment *string
	if v := request["tableComment"]; v != nil {
		str := fmt.Sprint(v)
		tableComment = &str
	}
	var synced *int
	if v, ok := request["synced"]; ok {
		n := 0
		if v == true {
			n = 1
		}
		synced = &n
	}
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := updateTable(ctx, tx, id, tableComment, synced, currentUsername(ctx))
		if err != nil {
			return err
		}
		if n == 0 {
			return framework.Biz("代码生成表不存在")
		}
		return nil
	})
}

func (s *Service) generate(ctx context.Context, tableID int64) (map[string]any, error) {
	table, err := s.ensureTable(ctx, tableID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tableName": table["tableName"], "message": "代码压缩包已准备就绪"}, nil
}

func (s *Service) download(ctx context.Context, tableID int64) ([]byte, error) {
	table, err := s.ensureTable(ctx, tableID)
	if err != nil {
		return nil, err
	}
	columns, err := selectColumns(ctx, s.db, tableID)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range renderFiles(table, columns) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: file.Path, Method: zip.Deflate, Modified: time.Now()})
		if err == nil {
			_, err = w.Write([]byte(file.Code))
		}
		if err != nil {
			return nil, framework.Biz("代码压缩包生成失败")
		}
	}
	if err := zw.Close(); err != nil {
		return nil, framework.Biz("代码压缩包生成失败")
	}
	return out.Bytes(), nil
}

func (s *Service) ensureTable(ctx context.Context, tableID int64) (framework.Row, error) {
	table, err := selectTableById(ctx, s.db, tableID)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, framework.Biz("代码生成表不存在")
	}
	return table, nil
}

// tableRow
func tableRow(dbTable framework.Row) framework.Row {
	tableName := framework.AsString(dbTable["tableName"])
	return framework.Row{
		"tableName":    tableName,
		"tableComment": dbTable["tableComment"],
		"className":    upperCamel(tableName),
		"module":       moduleName(tableName),
		"businessName": businessName(tableName),
	}
}

// columnRow
func columnRow(tableID int64, column framework.Row) framework.Row {
	columnName := framework.AsString(column["columnName"])
	dataType := strings.ToLower(framework.AsString(column["dataType"]))
	primaryLike := columnName == "id"
	queryType := "="
	if queryField(columnName) {
		queryType = "LIKE"
	}
	return framework.Row{
		"tableId":       tableID,
		"columnName":    columnName,
		"javaField":     lowerCamel(columnName),
		"javaType":      javaType(dataType),
		"jdbcType":      dataType,
		"columnComment": column["columnComment"],
		"insert":        flag(!primaryLike),
		"edit":          flag(!primaryLike),
		"list":          flag(!primaryLike),
		"query":         flag(queryField(columnName)),
		"queryType":     queryType,
		"formType":      formType(dataType),
		"required":      flag(column["nullable"] == "NO" && !primaryLike),
		"sort":          column["sort"],
	}
}

func flag(b bool) int {
	if b {
		return 1
	}
	return 0
}

// booleanRows：Number → intValue()==1。
func booleanRows(rows []framework.Row, keys ...string) []framework.Row {
	for _, row := range rows {
		for _, key := range keys {
			if n, ok := row[key].(int64); ok {
				row[key] = n == 1
			}
		}
	}
	return rows
}

// currentUsername：无认证时回落为 "system"。
func currentUsername(ctx context.Context) string {
	if u := framework.CurrentUsername(ctx); u != "" {
		return u
	}
	return "system"
}

// ---------- GeneratorController ----------

type handlers struct{ s *Service }

func (h handlers) tables(w http.ResponseWriter, r *http.Request) error {
	q, err := framework.ParsePage(r)
	if err != nil {
		return err
	}
	page, err := h.s.tables(r.Context(), q, framework.QueryString(r, "keyword"))
	if err != nil {
		return err
	}
	return framework.OK(w, r, page)
}

func (h handlers) importableTables(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.s.importableTables(r.Context(), framework.QueryString(r, "keyword"))
	if err != nil {
		return err
	}
	return framework.OK(w, r, rows)
}

func (h handlers) importTables(w http.ResponseWriter, r *http.Request) error {
	var req map[string]any
	if err := framework.Bind(r, &req); err != nil {
		return err
	}
	v := req["tableNames"]
	names, ok := v.([]any)
	if v != nil && !ok {
		return errors.New("tableNames 不是数组") // Java: ClassCastException → 500
	}
	resp, err := h.s.importTables(r.Context(), names)
	if err != nil {
		return err
	}
	return framework.OK(w, r, resp)
}

func (h handlers) columns(w http.ResponseWriter, r *http.Request) error {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return err
	}
	rows, err := h.s.columns(r.Context(), id)
	if err != nil {
		return err
	}
	return framework.OK(w, r, rows)
}

func (h handlers) preview(w http.ResponseWriter, r *http.Request) error {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return err
	}
	resp, err := h.s.preview(r.Context(), id)
	if err != nil {
		return err
	}
	return framework.OK(w, r, resp)
}

func (h handlers) update(w http.ResponseWriter, r *http.Request) error {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return err
	}
	var req map[string]any
	if err := framework.Bind(r, &req); err != nil {
		return err
	}
	if err := h.s.updateTable(r.Context(), id, req); err != nil {
		return err
	}
	return framework.OK(w, r, nil)
}

func (h handlers) generate(w http.ResponseWriter, r *http.Request) error {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return err
	}
	resp, err := h.s.generate(r.Context(), id)
	if err != nil {
		return err
	}
	return framework.OK(w, r, resp)
}

func (h handlers) download(w http.ResponseWriter, r *http.Request) error {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return err
	}
	table, err := h.s.ensureTable(r.Context(), id)
	if err != nil {
		return err
	}
	className := framework.AsString(table["className"])
	body, err := h.s.download(r.Context(), id)
	if err != nil {
		return err
	}
	return framework.Attachment(w, "application/zip", className+".zip", body)
}

// RegisterRoutes 对应 GeneratorController 的 @RequestMapping("/api/tool/generator")。
func RegisterRoutes(mux *http.ServeMux, db *sql.DB, sec *framework.Security, rec framework.OperationLogRecorder) {
	h := handlers{s: &Service{db: db}}
	perm := sec.Perm("tool:gen:list")
	mux.Handle("GET /api/tool/generator/tables", framework.Handle(h.tables, perm))
	mux.Handle("GET /api/tool/generator/importable-tables", framework.Handle(h.importableTables, perm))
	mux.Handle("POST /api/tool/generator/import", framework.Handle(h.importTables, framework.OperLog(rec, "代码生成", "导入表"), perm))
	mux.Handle("GET /api/tool/generator/tables/{id}/columns", framework.Handle(h.columns, perm))
	mux.Handle("GET /api/tool/generator/tables/{id}/preview", framework.Handle(h.preview, perm))
	mux.Handle("PUT /api/tool/generator/tables/{id}", framework.Handle(h.update, framework.OperLog(rec, "代码生成", "修改配置"), perm))
	mux.Handle("POST /api/tool/generator/tables/{id}/generate", framework.Handle(h.generate, framework.OperLog(rec, "代码生成", "生成代码"), perm))
	mux.Handle("GET /api/tool/generator/tables/{id}/download", framework.Handle(h.download, framework.OperLog(rec, "代码生成", "下载代码"), perm))
}
