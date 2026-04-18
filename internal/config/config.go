package config

import (
	"strconv"
	"strings"
	"time"
)

type BootstrapAdminConfig struct {
	TenantID    string
	Username    string
	Email       string
	Password    string
	DisplayName string
}

type LoggingFileConfig struct {
	Enabled     bool
	Dir         string
	DailyRotate bool
}

type LoggingConfig struct {
	Level     string
	Format    string
	Stdout    bool
	Color     bool
	AddSource bool
	File      LoggingFileConfig
}

type DatabaseConfig struct {
	Driver          string
	DSN             string
	AutoMigrate     bool
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Enabled      bool
	Addr         string
	Username     string
	Password     string
	DB           int
	KeyPrefix    string
	PrincipalTTL time.Duration
}

type NacosConfig struct {
	Enabled     bool
	ServerAddrs []string
	NamespaceID string
	Group       string
	DataID      string
	Username    string
	Password    string
	Format      string
	Timeout     time.Duration
	LogDir      string
	CacheDir    string
}

type UploadConfig struct {
	Dir     string
	MaxSize int64  // 单位：bytes，0 表示不限制
	Backend string // "local"（默认）或 "minio"
}

type MinioConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	BucketName      string
	Region          string
}

type CommonConfig struct {
	Nacos               NacosConfig
	Database            DatabaseConfig
	Redis               RedisConfig
	Upload              UploadConfig
	Minio               MinioConfig
	TokenSecret         string
	TokenTTL            time.Duration
	RefreshTokenTTL     time.Duration
	CredentialSecretKey string
	CORSAllowedOrigins  []string
	AllowDirectToken    bool
	BootstrapAdmin      BootstrapAdminConfig
	Logging             LoggingConfig
	ReadHeaderTimeout   time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	ShutdownGracePeriod time.Duration
}

type AuthServiceConfig struct {
	CommonConfig
	HTTPAddr string
}

type AppServiceConfig struct {
	CommonConfig
	HTTPAddr string
}

func LoadAuthServiceConfig() AuthServiceConfig {
	return AuthServiceConfig{
		CommonConfig: loadCommonConfig(),
		HTTPAddr:     getEnv("PLAYGROUND_AUTH_HTTP_ADDR", ":8081"),
	}
}

func LoadAppServiceConfig() AppServiceConfig {
	return AppServiceConfig{
		CommonConfig: loadCommonConfig(),
		HTTPAddr:     getEnv("PLAYGROUND_APP_HTTP_ADDR", ":8090"),
	}
}

func loadCommonConfig() CommonConfig {
	return CommonConfig{
		Nacos: NacosConfig{
			Enabled:     getBool("PLAYGROUND_NACOS_ENABLED", false),
			ServerAddrs: getList("PLAYGROUND_NACOS_SERVER_ADDRS", []string{"http://127.0.0.1:8848"}),
			NamespaceID: getEnv("PLAYGROUND_NACOS_NAMESPACE_ID", ""),
			Group:       getEnv("PLAYGROUND_NACOS_GROUP", "DEFAULT_GROUP"),
			DataID:      getEnv("PLAYGROUND_NACOS_DATA_ID", "playground-backend.properties"),
			Username:    getEnv("PLAYGROUND_NACOS_USERNAME", "nacos"),
			Password:    getEnv("PLAYGROUND_NACOS_PASSWORD", "nacos"),
			Format:      getEnv("PLAYGROUND_NACOS_FORMAT", "properties"),
			Timeout:     getDuration("PLAYGROUND_NACOS_TIMEOUT", 5*time.Second),
			LogDir:      getEnv("PLAYGROUND_NACOS_LOG_DIR", "storage/nacos/log"),
			CacheDir:    getEnv("PLAYGROUND_NACOS_CACHE_DIR", "storage/nacos/cache"),
		},
		Database: DatabaseConfig{
			Driver:          getEnv("PLAYGROUND_DB_DRIVER", "mysql"),
			DSN:             getEnv("PLAYGROUND_DB_DSN", "root:root@tcp(127.0.0.1:3306)/playground?parseTime=true&charset=utf8mb4&loc=Local"),
			AutoMigrate:     getBool("PLAYGROUND_DB_AUTO_MIGRATE", true),
			MaxOpenConns:    getInt("PLAYGROUND_DB_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getInt("PLAYGROUND_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDuration("PLAYGROUND_DB_CONN_MAX_LIFETIME", 30*time.Minute),
		},
		Redis: RedisConfig{
			Enabled:      getBool("PLAYGROUND_REDIS_ENABLED", false),
			Addr:         getEnv("PLAYGROUND_REDIS_ADDR", "127.0.0.1:6379"),
			Username:     getEnv("PLAYGROUND_REDIS_USERNAME", ""),
			Password:     getEnv("PLAYGROUND_REDIS_PASSWORD", ""),
			DB:           getInt("PLAYGROUND_REDIS_DB", 0),
			KeyPrefix:    getEnv("PLAYGROUND_REDIS_KEY_PREFIX", "playground"),
			PrincipalTTL: getDuration("PLAYGROUND_REDIS_PRINCIPAL_TTL", 10*time.Minute),
		},
		Upload: UploadConfig{
			Dir:     getEnv("PLAYGROUND_UPLOAD_DIR", "storage/uploads"),
			MaxSize: int64(getInt("PLAYGROUND_UPLOAD_MAX_SIZE_MB", 100)) * 1024 * 1024,
			Backend: getEnv("PLAYGROUND_UPLOAD_BACKEND", "local"),
		},
		Minio: MinioConfig{
			Endpoint:        getEnv("PLAYGROUND_MINIO_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("PLAYGROUND_MINIO_ACCESS_KEY", "minioadmin"),
			SecretAccessKey: getEnv("PLAYGROUND_MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:          getBool("PLAYGROUND_MINIO_USE_SSL", false),
			BucketName:      getEnv("PLAYGROUND_MINIO_BUCKET", "playground-uploads"),
			Region:          getEnv("PLAYGROUND_MINIO_REGION", "us-east-1"),
		},
		TokenSecret:         getEnv("PLAYGROUND_TOKEN_SECRET", "change-me-before-production"),
		TokenTTL:            getDuration("PLAYGROUND_TOKEN_TTL", 24*time.Hour),
		RefreshTokenTTL:     getDuration("PLAYGROUND_REFRESH_TOKEN_TTL", 168*time.Hour),
		CredentialSecretKey: getEnv("PLAYGROUND_CREDENTIAL_SECRET_KEY", getEnv("PLAYGROUND_TOKEN_SECRET", "change-me-before-production")),
		CORSAllowedOrigins:  getList("PLAYGROUND_CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost"}),
		AllowDirectToken:    getBool("PLAYGROUND_ALLOW_DIRECT_TOKEN", true),
		Logging: LoggingConfig{
			Level:     getEnv("PLAYGROUND_LOG_LEVEL", "info"),
			Format:    getEnv("PLAYGROUND_LOG_FORMAT", "text"),
			Stdout:    getBool("PLAYGROUND_LOG_STDOUT", true),
			Color:     getBool("PLAYGROUND_LOG_COLOR", true),
			AddSource: getBool("PLAYGROUND_LOG_ADD_SOURCE", false),
			File: LoggingFileConfig{
				Enabled:     getBool("PLAYGROUND_LOG_FILE_ENABLED", false),
				Dir:         getEnv("PLAYGROUND_LOG_FILE_DIR", "storage/logs"),
				DailyRotate: getBool("PLAYGROUND_LOG_FILE_DAILY_ROTATE", true),
			},
		},
		ReadHeaderTimeout:   getDuration("PLAYGROUND_READ_HEADER_TIMEOUT", 5*time.Second),
		WriteTimeout:        getDuration("PLAYGROUND_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:         getDuration("PLAYGROUND_IDLE_TIMEOUT", 60*time.Second),
		ShutdownGracePeriod: getDuration("PLAYGROUND_SHUTDOWN_GRACE_PERIOD", 10*time.Second),
		BootstrapAdmin: BootstrapAdminConfig{
			TenantID:    getEnv("PLAYGROUND_BOOTSTRAP_TENANT_ID", "default"),
			Username:    getEnv("PLAYGROUND_BOOTSTRAP_ADMIN_USERNAME", "admin"),
			Email:       getEnv("PLAYGROUND_BOOTSTRAP_ADMIN_EMAIL", "admin@example.com"),
			Password:    getEnv("PLAYGROUND_BOOTSTRAP_ADMIN_PASSWORD", "ChangeMe123!"),
			DisplayName: getEnv("PLAYGROUND_BOOTSTRAP_ADMIN_DISPLAY_NAME", "Playground Admin"),
		},
	}
}

func getEnv(key, fallback string) string {
	if value, ok := lookupValue(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value, ok := lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func getList(key string, fallback []string) []string {
	value, ok := lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	if len(items) == 0 {
		return fallback
	}

	return items
}

func getBool(key string, fallback bool) bool {
	value, ok := lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt(key string, fallback int) int {
	value, ok := lookupValue(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
