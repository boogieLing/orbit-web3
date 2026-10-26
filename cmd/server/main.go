//go:build !js

// 本地原生 API 入口复用发布版本的全部领域逻辑。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"usedit/internal/community"
	"usedit/internal/database"
	"usedit/internal/studio"
)

// main 初始化持久化并在退出时等待请求完成。
func main() {
	address := flag.String("addr", "127.0.0.1:18441", "HTTP监听地址")
	dbPath := flag.String("db", "data/usedit.sqlite", "SQLite文件")
	flag.Parse()
	if mkdirErr := os.MkdirAll(filepath.Dir(*dbPath), 0700); mkdirErr != nil {
		slog.Error("data_directory", "error", mkdirErr)
		return
	}
	db, openErr := database.Open(*dbPath)
	if openErr != nil {
		return
	}
	defer db.Conn.Close()
	if migrationErr := migrate(db); migrationErr != nil {
		slog.Error("migrate", "error", migrationErr)
		return
	}
	config := community.Config{Studio: studio.Config{URL: os.Getenv("STUDIO_URL"), Key: os.Getenv("STUDIO_API_KEY"), Model: os.Getenv("STUDIO_MODEL"), Provider: os.Getenv("STUDIO_PROVIDER")}, ExperienceMode: os.Getenv("USEDIT_EXPERIENCE_MODE") != "false", RPCURL: os.Getenv("ETH_RPC_URL"), ComposeURL: os.Getenv("COMPOSE_URL"), ComposeKey: os.Getenv("COMPOSE_API_KEY"), ComposeModel: os.Getenv("COMPOSE_MODEL")}
	if config.RPCURL == "" {
		config.RPCURL = "https://ethereum-rpc.publicnode.com"
	}
	service := community.New(db, config)
	// 原生入口只负责 HTTP 边界，身份、权限和存储规则继续由领域服务执行。
	server := &http.Server{Addr: *address, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 110 * time.Second, IdleTimeout: 60 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			http.NotFound(w, r)
			return
		}
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if readErr != nil {
			http.Error(w, "请求内容过长", 413)
			return
		}
		headers := map[string]string{}
		for key := range r.Header {
			headers[strings.ToLower(key)] = r.Header.Get(key)
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		result := service.Handle(r.Context(), community.Request{Method: r.Method, URL: scheme + "://" + r.Host + r.URL.RequestURI(), Headers: headers, Body: string(body)})
		for key, value := range result.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(result.Status)
		if _, writeErr := io.WriteString(w, result.Body); writeErr != nil {
			slog.Warn("http_write", "error", writeErr)
		}
	})}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			slog.Error("shutdown", "error", shutdownErr)
		}
	}()
	slog.Info("native_server", "url", "http://"+*address)
	if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
		slog.Error("serve", "error", serveErr)
	}
}

// migrate 仅在启动阶段按文件名应用不可变模式迁移。
func migrate(db *database.SQLite) error {
	if _, setupErr := db.Conn.Exec("CREATE TABLE IF NOT EXISTS local_migrations(name TEXT PRIMARY KEY)"); setupErr != nil {
		slog.Error("migration_journal", "error", setupErr)
		return setupErr
	}
	files, globErr := filepath.Glob("drizzle/*.sql")
	if globErr != nil {
		slog.Error("migration_files", "error", globErr)
		return globErr
	}
	if len(files) == 0 {
		slog.Error("migration_missing")
		return fmt.Errorf("请从项目根目录启动，以读取 drizzle 迁移")
	}
	// 启动期迁移按文件顺序执行，不能合并跨版本事务。
	for _, file := range files {
		var count int
		if queryErr := db.Conn.QueryRow("SELECT COUNT(*) FROM local_migrations WHERE name=?", file).Scan(&count); queryErr != nil {
			slog.Error("migration_lookup", "error", queryErr)
			return queryErr
		}
		if count > 0 {
			continue
		}
		source, readErr := os.ReadFile(file)
		if readErr != nil {
			slog.Error("migration_read", "error", readErr)
			return readErr
		}
		tx, beginErr := db.Conn.Begin()
		if beginErr != nil {
			slog.Error("migration_begin", "error", beginErr)
			return beginErr
		}
		if _, applyErr := tx.Exec(string(source)); applyErr != nil {
			tx.Rollback()
			slog.Error("migration_apply", "error", applyErr)
			return applyErr
		}
		if _, recordErr := tx.Exec("INSERT INTO local_migrations(name) VALUES(?)", file); recordErr != nil {
			tx.Rollback()
			slog.Error("migration_record", "error", recordErr)
			return recordErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			slog.Error("migration_commit", "error", commitErr)
			return commitErr
		}
	}
	return nil
}
