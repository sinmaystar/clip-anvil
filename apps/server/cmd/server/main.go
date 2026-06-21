package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/sinmaystar/clip-anvil/internal/api"
	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/config"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pgPool, err := pgxpool.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		slog.Error("failed to create postgres pool", "error", err)
		os.Exit(1)
	}
	if err := pgPool.Ping(ctx); err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("postgres connected")
	defer pgPool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	slog.Info("redis connected")
	defer func() { _ = rdb.Close() }()

	storageService, err := storage.New(cfg.MinIO)
	if err != nil {
		slog.Error("failed to create storage service", "error", err)
		os.Exit(1)
	}
	if _, err := storageService.ListBuckets(ctx); err != nil {
		slog.Error("failed to connect to minio", "error", err)
		os.Exit(1)
	}
	slog.Info("minio connected")

	h := server.Default(server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)))
	h.NoHijackConnPool = true
	queries := db.New(pgPool)
	sandboxClient := sandbox.NewOpenSandboxClient(cfg.Sandbox)
	sandboxStore := sandbox.NewStore(pgPool, queries)
	sandboxManager := sandbox.NewManager(sandboxClient, cfg.Sandbox, sandboxStore)
	authHandler := api.NewAuthHandler(queries, cfg.JWT.Secret, cfg.JWT.ExpireHours)
	authMiddleware := auth.Middleware(cfg.JWT.Secret)
	canvasHub := api.NewCanvasHub()
	workspaceHandler := api.NewWorkspaceHandler(pgPool, queries)
	canvasHandler := api.NewCanvasHandler(queries, storageService)
	nodeHandler := api.NewNodeHandler(pgPool, queries, canvasHub)
	providerRegistry := production.NewProviderRegistry(production.ProviderConfig{
		ProviderMode:     cfg.Production.ProviderMode,
		DefaultProvider:  cfg.Production.DefaultProvider,
		DefaultTextModel: cfg.Production.DefaultTextModel,
		Volcengine: production.VolcengineProviderConfig{
			APIKey:     cfg.Production.Volcengine.APIKey,
			BaseURL:    cfg.Production.Volcengine.BaseURL,
			Region:     cfg.Production.Volcengine.Region,
			TextModel:  cfg.Production.Volcengine.TextModel,
			ImageModel: cfg.Production.Volcengine.ImageModel,
			VideoModel: cfg.Production.Volcengine.VideoModel,
			AudioModel: cfg.Production.Volcengine.AudioModel,
		},
	})
	sandboxJobService := sandbox.NewJobService(sandboxManager, sandboxClient, queries, storageService)
	providerRegistry.Register("internal_ffmpeg", production.NewInternalFFmpegProvider(sandboxJobService))
	productionService := production.NewService(pgPool, queries, providerRegistry, storageService)
	productionService.SetRemoteAssetImporter(sandboxJobService)
	legacyProductionRuntime := production.NewLegacyProviderRuntime(providerRegistry)
	var productionRuntime production.EinoProductionRuntime = legacyProductionRuntime
	if cfg.Production.ProviderMode == "real" && cfg.Production.DefaultProvider == "volcengine" {
		var inputResolver production.ProviderInputResolver
		tosCfg := cfg.Production.Volcengine.TOS
		if strings.TrimSpace(tosCfg.AccessKeyID) != "" || strings.TrimSpace(tosCfg.SecretAccessKey) != "" {
			tosStore, err := production.NewTOSStagingStore(production.TOSStagingStoreConfig{
				AccessKeyID:     tosCfg.AccessKeyID,
				SecretAccessKey: tosCfg.SecretAccessKey,
				Bucket:          tosCfg.Bucket,
				Endpoint:        tosCfg.Endpoint,
				Region:          tosCfg.Region,
				PublicBaseURL:   tosCfg.PublicBaseURL,
			})
			if err != nil {
				slog.Error("failed to create volcengine tos staging store", "error", err)
				os.Exit(1)
			}
			inputResolver = production.NewTOSProviderAssetResolver(
				tosStore,
				http.DefaultClient,
				production.TOSProviderAssetResolverConfig{
					URLTTL: time.Duration(tosCfg.SignedURLTTLSeconds) * time.Second,
				},
				storageService,
			)
		}
		productionRuntime = production.NewVolcengineProductionRuntime(
			production.VolcengineProviderConfig{
				APIKey:     cfg.Production.Volcengine.APIKey,
				BaseURL:    cfg.Production.Volcengine.BaseURL,
				Region:     cfg.Production.Volcengine.Region,
				TextModel:  cfg.Production.Volcengine.TextModel,
				ImageModel: cfg.Production.Volcengine.ImageModel,
				VideoModel: cfg.Production.Volcengine.VideoModel,
				AudioModel: cfg.Production.Volcengine.AudioModel,
			},
			http.DefaultClient,
			time.Duration(cfg.Production.ProviderPollIntervalSeconds)*time.Second,
			time.Duration(cfg.Production.ProviderMaxPollSeconds)*time.Second,
			legacyProductionRuntime,
			inputResolver,
		)
	}
	productionRunner := production.NewProductionRunner(
		productionService,
		productionRuntime,
		cfg.Production.WorkerConcurrency,
		api.NewProductionBroadcaster(canvasHub),
	)
	productionService.SetRunner(productionRunner)
	productionRunner.Start(ctx)
	runHandler := api.NewRunHandler(productionService, queries, storageService)
	modelHandler := api.NewModelHandler(queries)
	referencePackHandler := api.NewReferencePackHandler(pgPool, queries, productionService)
	edgeHandler := api.NewEdgeHandler(pgPool, queries, canvasHub)
	groupHandler := api.NewGroupHandler(pgPool, queries, canvasHub)
	uploadHandler := api.NewUploadHandler(queries, storageService)
	storageHandler := api.NewStorageHandler(queries, storageService)
	canvasWSHandler := api.NewCanvasWSHandler(queries, canvasHub, cfg.JWT.Secret)
	artifactService := sandbox.NewArtifactService(
		sandboxClient,
		queries,
		storageService,
		api.NewSandboxBroadcaster(canvasHub),
	)
	sandboxHandler := api.NewSandboxHandler(queries, sandboxManager, sandboxClient, artifactService, storageService)
	sandboxHTTPClient := &http.Client{Timeout: 2 * time.Second}

	h.GET("/api/health", func(ctx context.Context, c *app.RequestContext) {
		pgStatus := "connected"
		if err := pgPool.Ping(ctx); err != nil {
			pgStatus = "disconnected"
		}

		redisStatus := "connected"
		if err := rdb.Ping(ctx).Err(); err != nil {
			redisStatus = "disconnected"
		}

		minioStatus := "connected"
		if _, err := storageService.ListBuckets(ctx); err != nil {
			minioStatus = "disconnected"
		}

		sandboxStatus := "connected"
		if err := checkSandboxServerHealth(ctx, sandboxHTTPClient, cfg.Sandbox.Endpoint); err != nil {
			sandboxStatus = "disconnected"
		}

		status := "ok"
		if pgStatus != "connected" || redisStatus != "connected" || minioStatus != "connected" || sandboxStatus != "connected" {
			status = "degraded"
		}

		c.JSON(consts.StatusOK, map[string]any{
			"status": status,
			"services": map[string]string{
				"postgres": pgStatus,
				"redis":    redisStatus,
				"minio":    minioStatus,
				"sandbox":  sandboxStatus,
			},
		})
	})

	authGroup := h.Group("/api/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.GET("/me", authMiddleware, authHandler.Me)

	h.POST("/api/workspaces", authMiddleware, workspaceHandler.Create)
	h.POST("/api/workspaces/", authMiddleware, workspaceHandler.Create)
	h.GET("/api/workspaces", authMiddleware, workspaceHandler.List)
	h.GET("/api/workspaces/", authMiddleware, workspaceHandler.List)
	h.GET("/api/workspaces/:id/canvas", authMiddleware, canvasHandler.GetCanvas)
	h.PATCH("/api/workspaces/:id/camera", authMiddleware, canvasHandler.UpdateCamera)
	h.GET("/api/workspaces/:id/sandbox", authMiddleware, sandboxHandler.Status)
	h.DELETE("/api/workspaces/:id/sandbox", authMiddleware, sandboxHandler.Delete)
	h.POST("/api/workspaces/:id/sandbox/exec", authMiddleware, sandboxHandler.Exec)
	h.POST("/api/workspaces/:id/sandbox/artifacts", authMiddleware, sandboxHandler.SubmitArtifact)
	h.POST("/api/workspaces/:id/sandbox/download-from-minio", authMiddleware, sandboxHandler.DownloadFromMinIO)
	h.POST("/api/workspaces/:id/sandbox/upload-to-minio", authMiddleware, sandboxHandler.UploadToMinIO)
	h.POST("/api/workspaces/:id/storage/upload", authMiddleware, storageHandler.Upload)
	h.POST("/api/workspaces/:id/storage/presigned-upload", authMiddleware, storageHandler.PresignedUpload)
	h.POST("/api/workspaces/:id/storage/complete-upload", authMiddleware, storageHandler.CompleteUpload)
	h.GET("/api/workspaces/:id", authMiddleware, workspaceHandler.Get)
	h.GET("/api/model-capabilities", authMiddleware, modelHandler.ListCapabilities)

	h.POST("/api/nodes", authMiddleware, nodeHandler.Create)
	h.POST("/api/nodes/", authMiddleware, nodeHandler.Create)
	h.PATCH("/api/nodes/batch-position", authMiddleware, nodeHandler.BatchUpdatePosition)
	h.POST("/api/nodes/:id/run", authMiddleware, runHandler.RunNode)
	h.GET("/api/nodes/:id/versions", authMiddleware, runHandler.ListNodeVersions)
	h.POST("/api/nodes/:id/versions/:versionID/select", authMiddleware, runHandler.SelectNodeVersion)
	h.GET("/api/nodes/:id/production-state", authMiddleware, runHandler.GetNodeProductionState)
	h.GET("/api/nodes/:id/jobs", authMiddleware, runHandler.ListNodeJobs)
	h.GET("/api/nodes/:id/stale-reasons", authMiddleware, runHandler.ListStaleReasons)
	h.GET("/api/nodes/:id/inputs", authMiddleware, nodeHandler.Inputs)
	h.GET("/api/nodes/:id", authMiddleware, nodeHandler.Get)
	h.PATCH("/api/nodes/:id", authMiddleware, nodeHandler.Update)
	h.DELETE("/api/nodes/:id", authMiddleware, nodeHandler.Delete)

	h.GET("/api/reference-packs/:id/items", authMiddleware, referencePackHandler.ListItems)
	h.PUT("/api/reference-packs/:id/items", authMiddleware, referencePackHandler.ReplaceItems)

	h.POST("/api/groups", authMiddleware, groupHandler.Create)
	h.POST("/api/groups/", authMiddleware, groupHandler.Create)
	h.PATCH("/api/groups/:id", authMiddleware, groupHandler.Update)
	h.DELETE("/api/groups/:id", authMiddleware, groupHandler.Delete)
	h.PUT("/api/groups/:id/nodes", authMiddleware, groupHandler.ReplaceNodes)

	h.POST("/api/upload", authMiddleware, uploadHandler.Upload)

	h.POST("/api/edges", authMiddleware, edgeHandler.Create)
	h.POST("/api/edges/", authMiddleware, edgeHandler.Create)
	h.DELETE("/api/edges/:id", authMiddleware, edgeHandler.Delete)
	h.GET("/api/jobs/:id", authMiddleware, runHandler.GetJob)
	h.GET("/api/jobs/:id/sandbox-jobs", authMiddleware, runHandler.ListJobSandboxJobs)
	h.POST("/api/jobs/:id/retry", authMiddleware, runHandler.RetryJob)
	h.GET("/api/sandbox-jobs/:id", authMiddleware, runHandler.GetSandboxJob)

	h.GET("/ws/canvas", canvasWSHandler.Canvas)

	slog.Info("server starting", "port", cfg.Server.Port)
	h.Spin()
}

func checkSandboxServerHealth(ctx context.Context, client *http.Client, endpoint string) error {
	healthURL, err := sandboxHealthURL(endpoint)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("sandbox health status %d", resp.StatusCode)
	}
	return nil
}

func sandboxHealthURL(endpoint string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	base = strings.TrimSuffix(base, "/v1")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid sandbox endpoint")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/health"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
