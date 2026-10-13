package generator

import (
	"context"

	"scaffold/internal/framework"
)

// 对应 GeneratorMapper：每条语句原样照搬，#{x} → ?，同名参数按出现次数重复绑定。

// countTables
func countTables(ctx context.Context, q framework.Querier, keyword *string) (int64, error) {
	return framework.SelectInt64(ctx, q, `
SELECT COUNT(*)
FROM gen_table
WHERE ? IS NULL OR ? = ''
   OR table_name LIKE CONCAT('%', ?, '%')
   OR table_comment LIKE CONCAT('%', ?, '%')
   OR class_name LIKE CONCAT('%', ?, '%')`, keyword, keyword, keyword, keyword, keyword)
}

// selectTables
func selectTables(ctx context.Context, q framework.Querier, keyword *string, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
SELECT id,
       table_name AS tableName,
       table_comment AS tableComment,
       class_name AS className,
       module_name AS module,
       business_name AS businessName,
       synced,
       DATE_FORMAT(create_time, '%Y-%m-%d %H:%i') AS createTime
FROM gen_table
WHERE ? IS NULL OR ? = ''
   OR table_name LIKE CONCAT('%', ?, '%')
   OR table_comment LIKE CONCAT('%', ?, '%')
   OR class_name LIKE CONCAT('%', ?, '%')
ORDER BY update_time DESC, id DESC
LIMIT ?, ?`, keyword, keyword, keyword, keyword, keyword, offset, pageSize)
}

// selectImportableTables
func selectImportableTables(ctx context.Context, q framework.Querier, keyword *string) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
SELECT t.table_name AS tableName,
       t.table_comment AS tableComment,
       CASE WHEN g.id IS NULL THEN 0 ELSE 1 END AS imported
FROM information_schema.tables t
LEFT JOIN gen_table g ON g.table_name = t.table_name
WHERE t.table_schema = DATABASE()
  AND t.table_type = 'BASE TABLE'
  AND (? IS NULL OR ? = ''
       OR t.table_name LIKE CONCAT('%', ?, '%')
       OR t.table_comment LIKE CONCAT('%', ?, '%'))
ORDER BY t.table_name`, keyword, keyword, keyword, keyword)
}

// selectDatabaseTable
func selectDatabaseTable(ctx context.Context, q framework.Querier, tableName any) (framework.Row, error) {
	return framework.SelectRow(ctx, q, `
SELECT table_name AS tableName,
       table_comment AS tableComment
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name = ?`, tableName)
}

// selectDatabaseColumns
func selectDatabaseColumns(ctx context.Context, q framework.Querier, tableName any) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
SELECT column_name AS columnName,
       data_type AS dataType,
       column_type AS columnType,
       column_comment AS columnComment,
       is_nullable AS nullable,
       ordinal_position AS sort
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = ?
ORDER BY ordinal_position`, tableName)
}

// selectTableById
func selectTableById(ctx context.Context, q framework.Querier, id int64) (framework.Row, error) {
	return framework.SelectRow(ctx, q, `
SELECT id,
       table_name AS tableName,
       table_comment AS tableComment,
       class_name AS className,
       module_name AS module,
       business_name AS businessName,
       synced
FROM gen_table
WHERE id = ?`, id)
}

// selectTableIdByName：无记录返回 nil（Java 返回 null 的 Long）。
func selectTableIdByName(ctx context.Context, q framework.Querier, tableName any) (*int64, error) {
	row, err := framework.SelectRow(ctx, q, `SELECT id FROM gen_table WHERE table_name = ?`, tableName)
	if err != nil || row == nil {
		return nil, err
	}
	return framework.AsInt64Ptr(row["id"]), nil
}

// insertTable：返回自增主键（@Options(useGeneratedKeys)）。
func insertTable(ctx context.Context, q framework.Querier, row framework.Row, username string) (int64, error) {
	return framework.Insert(ctx, q, `
INSERT INTO gen_table (table_name, table_comment, class_name, module_name, business_name, synced, create_by, update_by)
VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		row["tableName"], row["tableComment"], row["className"], row["module"], row["businessName"], username, username)
}

// insertColumn
func insertColumn(ctx context.Context, q framework.Querier, row framework.Row) error {
	_, err := framework.Exec(ctx, q, `
INSERT INTO gen_table_column (table_id, column_name, java_field, java_type, jdbc_type, column_comment,
    is_insert, is_edit, is_list, is_query, query_type, form_type, required, sort)
VALUES (?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?)`,
		row["tableId"], row["columnName"], row["javaField"], row["javaType"], row["jdbcType"], row["columnComment"],
		row["insert"], row["edit"], row["list"], row["query"], row["queryType"], row["formType"], row["required"], row["sort"])
	return err
}

// selectColumns
func selectColumns(ctx context.Context, q framework.Querier, tableID int64) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
SELECT id,
       column_name AS col,
       java_field AS javaField,
       java_type AS javaType,
       jdbc_type AS jdbcType,
       column_comment AS comment,
       is_insert AS `+"`insert`"+`,
       is_edit AS edit,
       is_list AS list,
       is_query AS query,
       query_type AS queryType,
       form_type AS formType,
       required
FROM gen_table_column
WHERE table_id = ?
ORDER BY sort, id`, tableID)
}

// updateTable：返回影响行数。
func updateTable(ctx context.Context, q framework.Querier, id int64, tableComment *string, synced *int, username string) (int64, error) {
	return framework.Exec(ctx, q, `
UPDATE gen_table
SET table_comment = COALESCE(?, table_comment),
    synced = COALESCE(?, synced),
    update_by = ?
WHERE id = ?`, tableComment, synced, username, id)
}
