package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	App      AppConfig      `yaml:"app"`
	Server   ServerConfig   `yaml:"server"`
	Worker   WorkerConfig   `yaml:"worker"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	Redis    RedisConfig    `yaml:"redis"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	JWT      JWTConfig      `yaml:"jwt"`
}

type WorkerConfig struct {
	MetricsPort int `yaml:"metrics_port"`
}

type AppConfig struct {
	Name      string `yaml:"name"`
	Env       string `yaml:"env"`        // development, production, test
	LogLevel  string `yaml:"log_level"`  // debug, info, warn, error
	LogFormat string `yaml:"log_format"` // text, json
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type MySQLConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	DBName          string        `yaml:"dbname"`
	Params          string        `yaml:"params"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type RabbitMQConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	VHost    string `yaml:"vhost"`
}

type JWTConfig struct {
	AccessSecret  string        `yaml:"access_secret"`
	RefreshSecret string        `yaml:"refresh_secret"`
	AccessTTL     time.Duration `yaml:"access_ttl"`
	RefreshTTL    time.Duration `yaml:"refresh_ttl"`
}

// DSN returns the MySQL Data Source Name.
func (m *MySQLConfig) DSN() string {
	params := m.Params
	if params == "" {
		params = "charset=utf8mb4&parseTime=True&loc=Local"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		m.User, m.Password, m.Host, m.Port, m.DBName, params)
}

// Addr returns the Redis address string.
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// URL returns the AMQP connection URL.
func (q *RabbitMQConfig) URL() string {
	vhost := q.VHost
	if vhost == "" {
		vhost = "/"
	}
	if !strings.HasPrefix(vhost, "/") {
		vhost = "/" + vhost
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d%s",
		q.User, q.Password, q.Host, q.Port, vhost)
}

// Load loads the configuration from a YAML file (if provided) and overrides it with environment variables.
// It also automatically loads .env file if present in the current working directory.
func Load(filePath string) (*Config, error) {
	// Silently load .env if present
	_ = godotenv.Load()

	if filePath == "" {
		filePath = os.Getenv("CONFIG_PATH")
	}

	cfg := &Config{
		App: AppConfig{
			Name:      "gorangefeed",
			Env:       "development",
			LogLevel:  "info",
			LogFormat: "text",
		},
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		Worker: WorkerConfig{
			MetricsPort: 9091,
		},
		MySQL: MySQLConfig{
			Host:            "127.0.0.1",
			Port:            3307,
			Params:          "charset=utf8mb4&parseTime=True&loc=Local",
			MaxOpenConns:    100,
			MaxIdleConns:    10,
			ConnMaxLifetime: time.Hour,
		},
		Redis: RedisConfig{
			Host:     "127.0.0.1",
			Port:     6380,
			DB:       0,
			PoolSize: 50,
		},
		RabbitMQ: RabbitMQConfig{
			Host:  "127.0.0.1",
			Port:  5673,
			User:  "guest",
			VHost: "/",
		},
		JWT: JWTConfig{
			AccessSecret:  "gorangefeed_access_default_secret_key",
			RefreshSecret: "gorangefeed_refresh_default_secret_key",
			AccessTTL:     2 * time.Hour,
			RefreshTTL:    7 * 24 * time.Hour,
		},
	}

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read config file error: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config yaml error: %w", err)
		}
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Validate configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if val := os.Getenv("APP_ENV"); val != "" {
		cfg.App.Env = val
	}
	if val := os.Getenv("APP_LOG_LEVEL"); val != "" {
		cfg.App.LogLevel = val
	}
	if val := os.Getenv("APP_LOG_FORMAT"); val != "" {
		cfg.App.LogFormat = val
	}

	if val := os.Getenv("SERVER_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Server.Port = p
		}
	}
	if val := os.Getenv("WORKER_METRICS_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Worker.MetricsPort = p
		}
	}

	// MySQL overrides
	if val := getEnvAny("MYSQL_HOST", "DB_HOST"); val != "" {
		cfg.MySQL.Host = val
	}
	if val := getEnvAny("MYSQL_PORT", "DB_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.MySQL.Port = p
		}
	}
	if val := getEnvAny("MYSQL_USER", "DB_USER"); val != "" {
		cfg.MySQL.User = val
	}
	if val := getEnvAny("MYSQL_PASSWORD", "DB_PASSWORD"); val != "" {
		cfg.MySQL.Password = val
	}
	if val := getEnvAny("MYSQL_DBNAME", "DB_NAME"); val != "" {
		cfg.MySQL.DBName = val
	}
	if val := os.Getenv("MYSQL_PARAMS"); val != "" {
		cfg.MySQL.Params = val
	}

	// Redis overrides
	if val := os.Getenv("REDIS_HOST"); val != "" {
		cfg.Redis.Host = val
	}
	if val := os.Getenv("REDIS_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Redis.Port = p
		}
	}
	if val := os.Getenv("REDIS_PASSWORD"); val != "" {
		cfg.Redis.Password = val
	}
	if val := os.Getenv("REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil {
			cfg.Redis.DB = db
		}
	}

	// RabbitMQ overrides
	if val := os.Getenv("RABBITMQ_HOST"); val != "" {
		cfg.RabbitMQ.Host = val
	}
	if val := os.Getenv("RABBITMQ_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.RabbitMQ.Port = p
		}
	}
	if val := os.Getenv("RABBITMQ_USER"); val != "" {
		cfg.RabbitMQ.User = val
	}
	if val := os.Getenv("RABBITMQ_PASSWORD"); val != "" {
		cfg.RabbitMQ.Password = val
	}
	if val := os.Getenv("RABBITMQ_VHOST"); val != "" {
		cfg.RabbitMQ.VHost = val
	}

	// JWT overrides
	if val := os.Getenv("JWT_ACCESS_SECRET"); val != "" {
		cfg.JWT.AccessSecret = val
	}
	if val := os.Getenv("JWT_REFRESH_SECRET"); val != "" {
		cfg.JWT.RefreshSecret = val
	}
}

func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Validate performs checks on configuration fields.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	// App validation
	env := strings.ToLower(cfg.App.Env)
	if env != "development" && env != "production" && env != "test" {
		return fmt.Errorf("invalid app.env: %q (expected development, production, test)", cfg.App.Env)
	}

	logLevel := strings.ToLower(cfg.App.LogLevel)
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		return fmt.Errorf("invalid app.log_level: %q (expected debug, info, warn, error)", cfg.App.LogLevel)
	}

	logFormat := strings.ToLower(cfg.App.LogFormat)
	if logFormat != "text" && logFormat != "json" {
		return fmt.Errorf("invalid app.log_format: %q (expected text, json)", cfg.App.LogFormat)
	}

	// Server validation
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d (must be between 1 and 65535)", cfg.Server.Port)
	}

	// Worker validation
	if cfg.Worker.MetricsPort <= 0 || cfg.Worker.MetricsPort > 65535 {
		return fmt.Errorf("invalid worker.metrics_port: %d (must be between 1 and 65535)", cfg.Worker.MetricsPort)
	}

	// MySQL validation
	if cfg.MySQL.Host == "" {
		return errors.New("mysql.host is required")
	}
	if cfg.MySQL.Port <= 0 || cfg.MySQL.Port > 65535 {
		return fmt.Errorf("invalid mysql.port: %d (must be between 1 and 65535)", cfg.MySQL.Port)
	}
	if cfg.MySQL.User == "" {
		return errors.New("mysql.user is required")
	}
	if cfg.MySQL.DBName == "" {
		return errors.New("mysql.dbname is required")
	}
	if cfg.MySQL.MaxOpenConns < 0 {
		return errors.New("mysql.max_open_conns cannot be negative")
	}
	if cfg.MySQL.MaxIdleConns < 0 {
		return errors.New("mysql.max_idle_conns cannot be negative")
	}
	if cfg.MySQL.MaxOpenConns > 0 && cfg.MySQL.MaxIdleConns > cfg.MySQL.MaxOpenConns {
		return errors.New("mysql.max_idle_conns cannot exceed max_open_conns")
	}

	// Prohibit clientFoundRows
	lowerParams := strings.ToLower(cfg.MySQL.Params)
	if strings.Contains(lowerParams, "clientfoundrows=true") || strings.Contains(lowerParams, "clientfoundrows=1") {
		return errors.New("mysql clientFoundRows=true is strictly prohibited: like and interaction flows rely on RowsAffected=0")
	}

	// Redis validation
	if cfg.Redis.Host == "" {
		return errors.New("redis.host is required")
	}
	if cfg.Redis.Port <= 0 || cfg.Redis.Port > 65535 {
		return fmt.Errorf("invalid redis.port: %d (must be between 1 and 65535)", cfg.Redis.Port)
	}
	if cfg.Redis.DB < 0 {
		return errors.New("redis.db cannot be negative")
	}
	if cfg.Redis.PoolSize < 0 {
		return errors.New("redis.pool_size cannot be negative")
	}

	// RabbitMQ validation
	if cfg.RabbitMQ.Host == "" {
		return errors.New("rabbitmq.host is required")
	}
	if cfg.RabbitMQ.Port <= 0 || cfg.RabbitMQ.Port > 65535 {
		return fmt.Errorf("invalid rabbitmq.port: %d (must be between 1 and 65535)", cfg.RabbitMQ.Port)
	}

	return nil
}
