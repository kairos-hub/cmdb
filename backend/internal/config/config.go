package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config 应用配置。加载优先级：环境变量 > 配置文件 > 内置默认值。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // debug | release
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	Charset  string `yaml:"charset"`
	MaxIdle  int    `yaml:"max_idle"`
	MaxOpen  int    `yaml:"max_open"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// DSN 返回 MySQL 连接串。
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.Name, d.Charset)
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8080, Mode: "release"},
		Database: DatabaseConfig{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "",
			Name: "cmdb", Charset: "utf8mb4", MaxIdle: 10, MaxOpen: 100,
		},
		JWT: JWTConfig{Secret: "replace-with-random-secret", ExpireHours: 24 * 30}, // 默认 30 天
		Log: LogConfig{Level: "info", Format: "text"},
	}
}

// Load 读取配置文件（可缺省），再用环境变量覆盖。
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("解析配置文件失败: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}
	cfg.overrideFromEnv()
	return cfg, nil
}

func (c *Config) overrideFromEnv() {
	envStr(&c.Server.Host, "CMDB_SERVER_HOST")
	envInt(&c.Server.Port, "CMDB_SERVER_PORT")
	envStr(&c.Server.Mode, "CMDB_SERVER_MODE")

	envStr(&c.Database.Host, "CMDB_DB_HOST")
	envInt(&c.Database.Port, "CMDB_DB_PORT")
	envStr(&c.Database.User, "CMDB_DB_USER")
	envStr(&c.Database.Password, "CMDB_DB_PASSWORD")
	envStr(&c.Database.Name, "CMDB_DB_NAME")

	envStr(&c.JWT.Secret, "CMDB_JWT_SECRET")
	envInt(&c.JWT.ExpireHours, "CMDB_JWT_EXPIRE_HOURS")

	envStr(&c.Log.Level, "CMDB_LOG_LEVEL")
	envStr(&c.Log.Format, "CMDB_LOG_FORMAT")
}

func envStr(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

func envInt(target *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}
