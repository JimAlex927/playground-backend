package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appaccount "playground/internal/application/account"
	appcredential "playground/internal/application/credential"
	apppermission "playground/internal/application/permission"
	approle "playground/internal/application/role"
	appupload "playground/internal/application/upload"
	"playground/internal/config"
	inframinio "playground/internal/infrastructure/minio"
	persistmysql "playground/internal/infrastructure/persistence/mysql"
	"playground/internal/infrastructure/security"
	localstorage "playground/internal/infrastructure/storage/local"
	miniostorage "playground/internal/infrastructure/storage/minio"
	apphttp "playground/internal/interfaces/http/app"
	"playground/internal/platform/httpx"
	"playground/internal/platform/logx"

	"go.uber.org/zap"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		panic(err)
	}
	nacosResult, err := config.LoadNacosOverrides()
	if err != nil {
		panic(err)
	}

	cfg := config.LoadAppServiceConfig()
	logger, cleanup, err := logx.New(logx.Config{
		ServiceName: "app-service",
		Level:       cfg.Logging.Level,
		Format:      cfg.Logging.Format,
		Stdout:      cfg.Logging.Stdout,
		Color:       cfg.Logging.Color,
		AddSource:   cfg.Logging.AddSource,
		File: logx.FileConfig{
			Enabled:     cfg.Logging.File.Enabled,
			Dir:         cfg.Logging.File.Dir,
			DailyRotate: cfg.Logging.File.DailyRotate,
		},
	})
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if nacosResult.Loaded {
		logger.Info("nacos config loaded",
			zap.String("server", nacosResult.Server),
			zap.String("group", nacosResult.Group),
			zap.String("dataId", nacosResult.DataID),
			zap.Int("itemCount", nacosResult.ItemCount),
		)
	} else if nacosResult.Enabled {
		logger.Info("nacos config enabled but no remote values loaded",
			zap.String("server", nacosResult.Server),
			zap.String("group", nacosResult.Group),
			zap.String("dataId", nacosResult.DataID),
		)
	} else {
		fmt.Println("Nacos config disabled, using env/.env/default values.")
	}

	db, err := persistmysql.Open(cfg.Database)
	if err != nil {
		logger.Error("open database failed", zap.Error(err))
		panic(err)
	}
	defer func() { _ = db.Close() }()

	if cfg.Database.AutoMigrate {
		if err := persistmysql.AutoMigrate(context.Background(), db); err != nil {
			logger.Error("auto migrate failed", zap.Error(err))
			panic(err)
		}
	}

	repo := persistmysql.NewAccountRepository(db)
	permissionRepo := persistmysql.NewPermissionRepository(db)
	roleRepo := persistmysql.NewRoleRepository(db)
	credentialRepo := persistmysql.NewCredentialRepository(db)
	hasher := security.NewPasswordHasher(120_000, 16, 32)
	fieldCipher, err := security.NewFieldCipher(cfg.CredentialSecretKey)
	if err != nil {
		logger.Error("init credential cipher failed", zap.Error(err))
		panic(err)
	}
	tokens := security.NewTokenManager(cfg.TokenSecret)
	permissionService := apppermission.NewService(permissionRepo, time.Now)
	roleService := approle.NewService(roleRepo, repo, permissionService, time.Now)
	accountService := appaccount.NewService(repo, roleRepo, hasher, time.Now)
	credentialService := appcredential.NewService(credentialRepo, fieldCipher, time.Now)

	// Upload：根据配置选择存储后端，只改这里，上层代码零改动。
	uploadRepo := persistmysql.NewUploadRepository(db)
	var tusHandler http.Handler
	switch cfg.Upload.Backend {
	case "minio":
		minioCfg := inframinio.Config{
			Endpoint:        cfg.Minio.Endpoint,
			AccessKeyID:     cfg.Minio.AccessKeyID,
			SecretAccessKey: cfg.Minio.SecretAccessKey,
			UseSSL:          cfg.Minio.UseSSL,
			BucketName:      cfg.Minio.BucketName,
			Region:          cfg.Minio.Region,
		}
		// 确保 bucket 存在（服务启动时检查一次）
		minioClient, err := inframinio.NewMinioClient(minioCfg)
		if err != nil {
			logger.Error("init minio client failed", zap.Error(err))
			panic(err)
		}
		if err := inframinio.EnsureBucket(context.Background(), minioClient, minioCfg); err != nil {
			logger.Error("ensure minio bucket failed", zap.Error(err))
			panic(err)
		}
		tusHandler, err = miniostorage.NewTusHandler(minioCfg, uploadRepo, cfg.Upload.MaxSize)
		if err != nil {
			logger.Error("init minio tus handler failed", zap.Error(err))
			panic(err)
		}
		logger.Info("upload backend: minio", zap.String("bucket", minioCfg.BucketName))
	default:
		tusHandler, err = localstorage.NewTusHandler(cfg.Upload.Dir, uploadRepo, cfg.Upload.MaxSize)
		if err != nil {
			logger.Error("init local tus handler failed", zap.Error(err))
			panic(err)
		}
		logger.Info("upload backend: local", zap.String("dir", cfg.Upload.Dir))
	}
	uploadService := appupload.NewService(uploadRepo)

	if _, err := permissionService.EnsureDefaultPermissions(context.Background(), cfg.BootstrapAdmin.TenantID); err != nil {
		logger.Error("ensure default permissions failed", zap.Error(err))
		panic(err)
	}

	defaultRoles, err := roleService.EnsureDefaultRoles(context.Background(), cfg.BootstrapAdmin.TenantID)
	if err != nil {
		logger.Error("ensure default roles failed", zap.Error(err))
		panic(err)
	}

	if _, created, err := accountService.EnsureBootstrapAdmin(context.Background(), appaccount.CreateAccountInput{
		TenantID:    cfg.BootstrapAdmin.TenantID,
		Username:    cfg.BootstrapAdmin.Username,
		Email:       cfg.BootstrapAdmin.Email,
		DisplayName: cfg.BootstrapAdmin.DisplayName,
		Password:    cfg.BootstrapAdmin.Password,
		RoleID:      defaultRoles["master"].ID,
		Status:      "active",
	}); err != nil {
		logger.Error("bootstrap admin failed", zap.Error(err))
		panic(err)
	} else if created {
		logger.Info("bootstrap admin created", zap.String("username", cfg.BootstrapAdmin.Username))
	}

	handler := httpx.Chain(
		apphttp.NewHandler(accountService, permissionService, roleService, credentialService, uploadService, tusHandler, tokens, cfg.AllowDirectToken),
		httpx.Recover(logger),
		httpx.RequestLogger(logger),
		httpx.CORS(cfg.CORSAllowedOrigins),
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	logger.Info("app service listening",
		zap.String("addr", cfg.HTTPAddr),
		zap.String("dbDriver", cfg.Database.Driver),
	)
	runServer(server, logger, cfg.ShutdownGracePeriod)
}

func runServer(server *http.Server, logger *zap.Logger, gracePeriod time.Duration) {
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", zap.Error(err))
			return
		}
	case <-stop:
		logger.Info("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), gracePeriod)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Error("server shutdown failed", zap.Error(err))
		}
	}
}
