package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// InitDB 对应 Java 的 DbInit：建库（不存在时）并按版本顺序导入 sqlDir 下的 V*__*.sql。
// 两份初始化脚本本身是幂等的（CREATE TABLE IF NOT EXISTS / INSERT IGNORE），容器每次启动执行也安全。
// MySQL 尚未就绪时最多等待 waitFor。
func InitDB(cfg Config, sqlDir string, waitFor time.Duration) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(cfg.DB.Name) {
		return fmt.Errorf("DB_NAME 只能包含字母、数字和下划线")
	}
	files, err := sqlFiles(sqlDir)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s:%s@tcp(%s:%s)/", cfg.DB.Username, cfg.DB.Password, cfg.DB.Host, cfg.DB.Port)
	params := "?charset=utf8mb4&multiStatements=true&time_zone=" + url.QueryEscape("'"+cfg.DB.TimeZone+"'")

	admin, err := openWithRetry(base+params, waitFor)
	if err != nil {
		return err
	}
	_, err = admin.Exec("CREATE DATABASE IF NOT EXISTS `" + cfg.DB.Name + "` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci")
	admin.Close()
	if err != nil {
		return fmt.Errorf("创建数据库失败: %w", err)
	}

	db, err := openWithRetry(base+cfg.DB.Name+params, waitFor)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		log.Printf("导入: %s", filepath.Base(f))
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("导入 %s 失败: %w", filepath.Base(f), err)
		}
	}
	log.Printf("数据库初始化完成（%d 个脚本）", len(files))
	return nil
}

var sqlVersion = regexp.MustCompile(`^V(\d+)__.*\.sql$`)

// sqlFiles 列出 V<n>__*.sql 并按 n 升序。
func sqlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取 SQL 目录 %s 失败: %w", dir, err)
	}
	type versioned struct {
		n    int
		path string
	}
	var list []versioned
	for _, e := range entries {
		if m := sqlVersion.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			list = append(list, versioned{n, filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n < list[j].n })
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = v.path
	}
	return out, nil
}

// openWithRetry 打开连接并在 waitFor 内重试 ping（容器编排中 MySQL 可能稍后才就绪）。
func openWithRetry(dsn string, waitFor time.Duration) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(waitFor)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			return db, nil
		}
		if time.Now().After(deadline) {
			db.Close()
			return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
		}
		log.Printf("等待 MySQL 就绪: %v", err)
		time.Sleep(2 * time.Second)
	}
}
