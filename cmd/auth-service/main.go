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
	appauth "playground/internal/application/auth"
	appeventing "playground/internal/application/eventing"
	apppermission "playground/internal/application/permission"
	approle "playground/internal/application/role"
	"playground/internal/config"
	rediscache "playground/internal/infrastructure/cache/redis"
	persistmysql "playground/internal/infrastructure/persistence/mysql"
	"playground/internal/infrastructure/security"
	authhttp "playground/internal/interfaces/http/auth"
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

	cfg := config.LoadAuthServiceConfig()
	logger, cleanup, err := logx.New(logx.Config{
		ServiceName: "auth-service",
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
	hasher := security.NewPasswordHasher(120_000, 16, 32)
	tokens := security.NewTokenManager(cfg.TokenSecret)
	redisClient, err := rediscache.NewClient(cfg.Redis)
	if err != nil {
		logger.Error("open redis failed", zap.Error(err))
		panic(err)
	}
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
	}
	principalCache := rediscache.NewPrincipalCache(redisClient, cfg.Redis)
	refreshStore := rediscache.NewRefreshSessionStore(redisClient, cfg.Redis)
	publisher := appeventing.NewInProcessPublisher(appauth.NewPrincipalCacheInvalidationHandler(principalCache))
	permissionService := apppermission.NewService(permissionRepo, time.Now)
	roleService := approle.NewService(roleRepo, repo, permissionService, publisher, time.Now)
	accountService := appaccount.NewService(repo, roleRepo, hasher, publisher, time.Now)

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

	authService := appauth.NewService(repo, hasher, tokens, principalCache, refreshStore, cfg.TokenTTL, cfg.RefreshTokenTTL)
	handler := httpx.Chain(
		authhttp.NewHandler(authService),
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

	logger.Info("auth service listening",
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
