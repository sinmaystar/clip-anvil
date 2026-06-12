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
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"github.com/sinmaystar/clip-anvil/internal/config"
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

	minioClient, err := minio.New(cfg.MinIO.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, ""),
		Secure: cfg.MinIO.UseSSL,
	})
	if err != nil {
		slog.Error("failed to create minio client", "error", err)
		os.Exit(1)
	}
	if _, err := minioClient.ListBuckets(ctx); err != nil {
		slog.Error("failed to connect to minio", "error", err)
		os.Exit(1)
	}
	slog.Info("minio connected")

	h := server.Default(server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)))

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
		if _, err := minioClient.ListBuckets(ctx); err != nil {
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

	slog.Info("server starting", "port", cfg.Server.Port)
	h.Spin()
}
