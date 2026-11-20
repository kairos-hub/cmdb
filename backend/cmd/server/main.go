package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cmdb/cmdb-backend/internal/auth"
	"github.com/cmdb/cmdb-backend/internal/config"
	"github.com/cmdb/cmdb-backend/internal/database"
	"github.com/cmdb/cmdb-backend/internal/router"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/logger"
	"github.com/gin-gonic/gin"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	skipInit := flag.Bool("skip-init", false, "跳过数据库迁移、初始化和后台任务")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)
	gin.SetMode(cfg.Server.Mode)

	db, err := database.New(cfg.Database)
	if err != nil {
		slog.Error("数据库初始化失败", "err", err)
		os.Exit(1)
	}

	// 装配依赖
	tokenManager := auth.NewTokenManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)
	userService := service.NewUserService(db)
	rbacService := service.NewRBACService(db)

	if !*skipInit {
		if err := database.Migrate(db); err != nil {
			slog.Error("数据库迁移失败", "err", err)
			os.Exit(1)
		}
		if err := database.Seed(db); err != nil {
			slog.Error("初始化内置数据失败", "err", err)
			os.Exit(1)
		}

		// 首次启动创建默认超级管理员
		created, err := userService.EnsureAdmin("admin", "admin123")
		if err != nil {
			slog.Error("初始化管理员失败", "err", err)
			os.Exit(1)
		}
		if created {
			slog.Warn("已创建默认超级管理员 admin / admin123，请尽快登录修改密码")
		}
		// 确保超管角色存在并绑定给 admin
		if err := rbacService.EnsureAdminBinding("admin"); err != nil {
			slog.Error("初始化超管角色失败", "err", err)
			os.Exit(1)
		}
	}

	r := router.New(db, userService, rbacService, tokenManager, !*skipInit)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		slog.Info("服务启动", "addr", addr, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务监听失败", "err", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("正在关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("服务关闭异常", "err", err)
	}
	slog.Info("服务已退出")
}
