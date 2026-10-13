// scaffold 后端 Go 版启动入口，对应 ScaffoldAdminApplication。
//
//	scaffold-server            启动服务；DB_AUTO_INIT=true 时先执行数据库初始化
//	scaffold-server init-db    仅执行数据库初始化（对应 Java 的 DbInit）
//
// SQL 目录由 SQL_DIR 指定，默认 ../scaffold-system/sql（本地开发）；镜像内为 /app/sql。
package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"scaffold/internal/app"
)

func main() {
	cfg := app.ConfigFromEnv()
	sqlDir := os.Getenv("SQL_DIR")
	if sqlDir == "" {
		sqlDir = "../scaffold-system/sql"
	}
	initOnly := len(os.Args) > 1 && os.Args[1] == "init-db"
	if initOnly || strings.EqualFold(os.Getenv("DB_AUTO_INIT"), "true") {
		if err := app.InitDB(cfg, sqlDir, 90*time.Second); err != nil {
			log.Fatalf("数据库初始化失败: %v", err)
		}
		if initOnly {
			return
		}
	}
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	defer a.Close()
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           a.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("scaffold-go 已启动，监听 :%s，接口前缀 /api", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
