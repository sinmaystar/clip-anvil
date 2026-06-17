package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/sinmaystar/clip-anvil/internal/api"
	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/config"
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
	canvasHandler := api.NewCanvasHandler(queries)
	nodeHandler := api.NewNodeHandler(pgPool, queries, canvasHub)
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

		status := "ok"
		if pgStatus != "connected" || redisStatus != "connected" || minioStatus != "connected" {
			status = "degraded"
		}

		c.JSON(consts.StatusOK, map[string]any{
			"status": status,
			"services": map[string]string{
				"postgres": pgStatus,
				"redis":    redisStatus,
				"minio":    minioStatus,
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

	h.POST("/api/nodes", authMiddleware, nodeHandler.Create)
	h.POST("/api/nodes/", authMiddleware, nodeHandler.Create)
	h.PATCH("/api/nodes/batch-position", authMiddleware, nodeHandler.BatchUpdatePosition)
	h.GET("/api/nodes/:id/inputs", authMiddleware, nodeHandler.Inputs)
	h.GET("/api/nodes/:id", authMiddleware, nodeHandler.Get)
	h.PATCH("/api/nodes/:id", authMiddleware, nodeHandler.Update)
	h.DELETE("/api/nodes/:id", authMiddleware, nodeHandler.Delete)

	h.POST("/api/groups", authMiddleware, groupHandler.Create)
	h.POST("/api/groups/", authMiddleware, groupHandler.Create)
	h.PATCH("/api/groups/:id", authMiddleware, groupHandler.Update)
	h.DELETE("/api/groups/:id", authMiddleware, groupHandler.Delete)
	h.PUT("/api/groups/:id/nodes", authMiddleware, groupHandler.ReplaceNodes)

	h.POST("/api/upload", authMiddleware, uploadHandler.Upload)

	h.POST("/api/edges", authMiddleware, edgeHandler.Create)
	h.POST("/api/edges/", authMiddleware, edgeHandler.Create)
	h.DELETE("/api/edges/:id", authMiddleware, edgeHandler.Delete)

	h.GET("/ws/canvas", canvasWSHandler.Canvas)

	slog.Info("server starting", "port", cfg.Server.Port)
	h.Spin()
}
