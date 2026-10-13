// Package app 对应 scaffold-admin：读取环境变量、装配依赖、注册路由。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"scaffold/internal/framework"
	"scaffold/internal/generator"
	"scaffold/internal/system"
)

// Config 与 application.yml / application-dev.yml 的环境变量一一对应。
type Config struct {
	Port           string
	JWTSecret      string
	JWTExpiresSecs int64
	DB             framework.DBConfig
	RedisAddr      string
	RedisPassword  string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv 读取进程环境变量（不加载 .env 文件，与 Java 版一致）。
func ConfigFromEnv() Config {
	expires, _ := strconv.ParseInt(env("JWT_EXPIRES_SECONDS", "7200"), 10, 64)
	return Config{
		Port:           env("SERVER_PORT", "8080"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTExpiresSecs: expires,
		DB: framework.DBConfig{
			Host:     env("DB_HOST", "localhost"),
			Port:     env("DB_PORT", "3306"),
			Name:     env("DB_NAME", "scaffold_system"),
			Username: env("DB_USERNAME", "root"),
			Password: env("DB_PASSWORD", "root"),
			TimeZone: env("DB_TIMEZONE", "+08:00"),
		},
		RedisAddr:     env("REDIS_HOST", "localhost") + ":" + env("REDIS_PORT", "6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}
}

// App 持有已装配的 HTTP 处理器与底层连接。
type App struct {
	Handler http.Handler
	DB      *sql.DB
	Redis   *redis.Client
}

// New 装配全部依赖；JWT 密钥缺失或不足 32 字节时返回错误并阻止启动。
func New(cfg Config) (*App, error) {
	jwt, err := framework.NewJWT(cfg.JWTSecret, cfg.JWTExpiresSecs)
	if err != nil {
		return nil, err
	}
	db, err := framework.OpenDB(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}

	perm := system.NewPermissionResolver(db)
	sec := &framework.Security{
		JWT:        jwt,
		Tokens:     framework.NewTokenStore(rdb),
		Permission: perm,
		PermitAll:  map[string]bool{"/api/health": true, "/api/auth/captcha": true, "/api/auth/login": true},
	}
	deps := system.Deps{
		DB:        db,
		Security:  sec,
		Recorder:  system.NewOperationLogRecorder(db),
		DataScope: system.NewDataScopeResolver(db),
		Perm:      perm,
	}

	mux := http.NewServeMux()
	// HealthController
	mux.Handle("GET /api/health", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		return framework.OK(w, r, map[string]any{"status": "UP", "time": time.Now().Format(time.RFC3339Nano)})
	}))
	system.RegisterAuthRoutes(mux, deps, system.NewAuthService(db, jwt, sec.Tokens), framework.NewCaptcha(rdb))
	system.RegisterManagementRoutes(mux, deps, system.NewManagementService(deps))
	system.RegisterDashboardRoutes(mux, system.NewDashboardService(db))
	system.RegisterMonitorStatsRoutes(mux, deps, system.NewMonitorStatsService(db))
	generator.RegisterRoutes(mux, db, sec, deps.Recorder)

	// 过滤器顺序与 Java 一致：TraceId → 异常兜底 → JWT 认证 → 路由（404/405 统一 JSON）
	handler := framework.Trace(framework.Recover(sec.Authenticate(framework.NotFoundOr405(mux))))
	return &App{Handler: handler, DB: db, Redis: rdb}, nil
}

// Close 释放连接。
func (a *App) Close() {
	_ = a.Redis.Close()
	_ = a.DB.Close()
}
