package config

import (
	"os"
	"path/filepath"
	"testing"
)

func validBaseConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:      "test_app",
			Env:       "development",
			LogLevel:  "info",
			LogFormat: "json",
		},
		Server: ServerConfig{
			Port: 8080,
		},
		MySQL: MySQLConfig{
			Host:   "127.0.0.1",
			Port:   3307,
			User:   "root",
			DBName: "gorangefeed_test",
			Params: "charset=utf8mb4&parseTime=True&loc=Local",
		},
		Redis: RedisConfig{
			Host: "127.0.0.1",
			Port: 6380,
			DB:   0,
		},
		RabbitMQ: RabbitMQConfig{
			Host: "127.0.0.1",
			Port: 5673,
		},
	}
}

func TestConfigValidation_Success(t *testing.T) {
	cfg := validBaseConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidation_RequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		modifyFn  func(c *Config)
		errSubstr string
	}{
		{
			name: "missing mysql host",
			modifyFn: func(c *Config) {
				c.MySQL.Host = ""
			},
			errSubstr: "mysql.host is required",
		},
		{
			name: "missing mysql user",
			modifyFn: func(c *Config) {
				c.MySQL.User = ""
			},
			errSubstr: "mysql.user is required",
		},
		{
			name: "missing mysql dbname",
			modifyFn: func(c *Config) {
				c.MySQL.DBName = ""
			},
			errSubstr: "mysql.dbname is required",
		},
		{
			name: "missing redis host",
			modifyFn: func(c *Config) {
				c.Redis.Host = ""
			},
			errSubstr: "redis.host is required",
		},
		{
			name: "missing rabbitmq host",
			modifyFn: func(c *Config) {
				c.RabbitMQ.Host = ""
			},
			errSubstr: "rabbitmq.host is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.modifyFn(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
			}
		})
	}
}

func TestConfigValidation_IllegalValues(t *testing.T) {
	tests := []struct {
		name      string
		modifyFn  func(c *Config)
		errSubstr string
	}{
		{
			name: "invalid server port",
			modifyFn: func(c *Config) {
				c.Server.Port = 70000
			},
			errSubstr: "invalid server.port",
		},
		{
			name: "invalid log level",
			modifyFn: func(c *Config) {
				c.App.LogLevel = "trace_all"
			},
			errSubstr: "invalid app.log_level",
		},
		{
			name: "invalid log format",
			modifyFn: func(c *Config) {
				c.App.LogFormat = "xml"
			},
			errSubstr: "invalid app.log_format",
		},
		{
			name: "invalid app env",
			modifyFn: func(c *Config) {
				c.App.Env = "staging"
			},
			errSubstr: "invalid app.env",
		},
		{
			name: "invalid mysql port",
			modifyFn: func(c *Config) {
				c.MySQL.Port = -1
			},
			errSubstr: "invalid mysql.port",
		},
		{
			name: "mysql idle conns > open conns",
			modifyFn: func(c *Config) {
				c.MySQL.MaxOpenConns = 10
				c.MySQL.MaxIdleConns = 20
			},
			errSubstr: "max_idle_conns cannot exceed max_open_conns",
		},
		{
			name: "negative redis db",
			modifyFn: func(c *Config) {
				c.Redis.DB = -1
			},
			errSubstr: "redis.db cannot be negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.modifyFn(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
			}
		})
	}
}

func TestConfigValidation_ProhibitClientFoundRows(t *testing.T) {
	tests := []struct {
		name   string
		params string
	}{
		{
			name:   "contains clientFoundRows=true",
			params: "charset=utf8mb4&parseTime=True&clientFoundRows=true",
		},
		{
			name:   "contains clientFoundRows=TRUE (uppercase)",
			params: "charset=utf8mb4&clientFoundRows=TRUE",
		},
		{
			name:   "contains clientFoundRows=1",
			params: "clientFoundRows=1&parseTime=True",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.MySQL.Params = tc.params
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("expected error for params %q, got nil", tc.params)
			}
		})
	}
}

func TestConfig_EnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
app:
  name: "gorangefeed"
  env: "development"
  log_level: "info"
  log_format: "text"
server:
  port: 8080
mysql:
  host: "localhost"
  port: 3307
  user: "root"
  password: "oldpassword"
  dbname: "gorangefeed"
redis:
  host: "127.0.0.1"
  port: 6380
rabbitmq:
  host: "127.0.0.1"
  port: 5673
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write temp yaml failed: %v", err)
	}

	os.Setenv("APP_ENV", "production")
	os.Setenv("APP_LOG_LEVEL", "warn")
	os.Setenv("APP_LOG_FORMAT", "json")
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("DB_HOST", "192.168.1.100")
	os.Setenv("DB_PORT", "3308")
	os.Setenv("DB_USER", "prod_user")
	os.Setenv("DB_PASSWORD", "new_secret_pwd")
	os.Setenv("DB_NAME", "prod_db")
	os.Setenv("REDIS_HOST", "redis.prod")
	os.Setenv("REDIS_PORT", "6379")
	os.Setenv("RABBITMQ_HOST", "mq.prod")
	os.Setenv("RABBITMQ_PORT", "5672")

	defer func() {
		os.Unsetenv("APP_ENV")
		os.Unsetenv("APP_LOG_LEVEL")
		os.Unsetenv("APP_LOG_FORMAT")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("REDIS_HOST")
		os.Unsetenv("REDIS_PORT")
		os.Unsetenv("RABBITMQ_HOST")
		os.Unsetenv("RABBITMQ_PORT")
	}()

	cfg, err := Load(yamlPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if cfg.App.Env != "production" {
		t.Errorf("expected App.Env production, got %s", cfg.App.Env)
	}
	if cfg.App.LogLevel != "warn" {
		t.Errorf("expected App.LogLevel warn, got %s", cfg.App.LogLevel)
	}
	if cfg.App.LogFormat != "json" {
		t.Errorf("expected App.LogFormat json, got %s", cfg.App.LogFormat)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected Server.Port 9090, got %d", cfg.Server.Port)
	}
	if cfg.MySQL.Host != "192.168.1.100" || cfg.MySQL.Port != 3308 || cfg.MySQL.User != "prod_user" || cfg.MySQL.DBName != "prod_db" {
		t.Errorf("MySQL config override failed: %+v", cfg.MySQL)
	}
	if cfg.Redis.Host != "redis.prod" || cfg.Redis.Port != 6379 {
		t.Errorf("Redis config override failed: %+v", cfg.Redis)
	}
	if cfg.RabbitMQ.Host != "mq.prod" || cfg.RabbitMQ.Port != 5672 {
		t.Errorf("RabbitMQ config override failed: %+v", cfg.RabbitMQ)
	}
}
