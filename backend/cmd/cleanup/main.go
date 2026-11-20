// 一次性清理命令：删除旧版「属于业务(belong_*_biz)」关联及其实例关联数据。
// 用法：go run ./cmd/cleanup -config config.yaml   （或编译后 ./cleanup -config config.yaml）
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cmdb/cmdb-backend/internal/config"
	"github.com/cmdb/cmdb-backend/internal/database"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "数据库连接失败: %v\n", err)
		os.Exit(1)
	}

	rels, assocs, err := database.CleanupLegacyBelong(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "清理失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("清理完成：删除「属于业务」实例关联 %d 条，模型关联定义 %d 条。\n", rels, assocs)
}
