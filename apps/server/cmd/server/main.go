package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	agentaudio "github.com/sinmaystar/clip-anvil/internal/agent/audio"
	agentcomposer "github.com/sinmaystar/clip-anvil/internal/agent/composer"
	agentcontextcompact "github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentcraftsman "github.com/sinmaystar/clip-anvil/internal/agent/craftsman"
	agentcreative "github.com/sinmaystar/clip-anvil/internal/agent/creative"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenthitl "github.com/sinmaystar/clip-anvil/internal/agent/hitl"
	"github.com/sinmaystar/clip-anvil/internal/agent/modelselection"
	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
	agentreferencevideo "github.com/sinmaystar/clip-anvil/internal/agent/referencevideo"
	agentrenderplan "github.com/sinmaystar/clip-anvil/internal/agent/renderplan"
	agentreviewer "github.com/sinmaystar/clip-anvil/internal/agent/reviewer"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentskills "github.com/sinmaystar/clip-anvil/internal/agent/skills"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	agentworker "github.com/sinmaystar/clip-anvil/internal/agent/worker"
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
	if err := validateRealMediaE2EConfig(cfg); err != nil {
		slog.Error("real media E2E configuration is invalid", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	agentTracing := initAgentTracing(ctx, slog.Default())
	defer func() {
		if err := agentTracing.Shutdown(context.Background()); err != nil {
			slog.Warn("failed to shutdown cozeloop agent tracing", "error", err)
		}
	}()

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
	agentHub := api.NewAgentHub()
	agentRuntime, err := agentruntime.NewService(pgPool, queries)
	if err != nil {
		slog.Error("failed to create agent runtime", "error", err)
		os.Exit(1)
	}
	agentEinoCheckpointStore := agenteino.NewCheckpointStore(agentRuntime, slog.Default().With("component", "eino_checkpoint_store"))
	agentGraphInfoRegistry := agenteino.NewGraphInfoRegistry()
	agentModelSelection := modelselection.NewService(queries, modelselection.Defaults{
		ProducerProviderID: "volcengine",
		ProducerModelID:    cfg.Production.Volcengine.TextModel,
	})
	agentBroadcaster := api.NewAgentBroadcaster(agentHub)
	hitlService := agenthitl.NewService(agentRuntime, agentBroadcaster)
	producerPSSBuilder := agentpss.NewBuilder(queries)
	creativeStateService := agentcreative.NewService(queries)
	audioPlanService := agentaudio.NewService(queries)
	renderPlanService := agentrenderplan.NewService(queries, agentrenderplan.NewPromptCompiler())
	referenceVideoAnalyzer := agentreferencevideo.NewVolcengineAnalyzer(
		agentreferencevideo.VolcengineAnalyzerConfig{
			APIKey:  cfg.Production.Volcengine.APIKey,
			BaseURL: cfg.Production.Volcengine.BaseURL,
			Region:  cfg.Production.Volcengine.Region,
			Model:   cfg.Production.Volcengine.TextModel,
		},
		agentreferencevideo.NewArkVideoAnalysisClient(
			cfg.Production.Volcengine.APIKey,
			cfg.Production.Volcengine.BaseURL,
			cfg.Production.Volcengine.Region,
		),
	)
	referenceVideoAnalysisService := agentreferencevideo.NewService(queries, referenceVideoAnalyzer).WithSourceURLSigner(storageService)
	workspaceHandler := api.NewWorkspaceHandler(pgPool, queries)
	canvasHandler := api.NewCanvasHandler(queries, storageService)
	nodeHandler := api.NewNodeHandler(pgPool, queries, canvasHub)
	providerRegistry := production.NewProviderRegistry(production.ProviderConfig{
		ProviderMode:     cfg.Production.ProviderMode,
		DefaultProvider:  cfg.Production.DefaultProvider,
		DefaultTextModel: cfg.Production.DefaultTextModel,
		Volcengine: production.VolcengineProviderConfig{
			APIKey:                  cfg.Production.Volcengine.APIKey,
			AudioAPIKey:             cfg.Production.Volcengine.AudioAPIKey,
			BaseURL:                 cfg.Production.Volcengine.BaseURL,
			AudioBaseURL:            cfg.Production.Volcengine.AudioBaseURL,
			Region:                  cfg.Production.Volcengine.Region,
			TextModel:               cfg.Production.Volcengine.TextModel,
			ImageModel:              cfg.Production.Volcengine.ImageModel,
			VideoModel:              cfg.Production.Volcengine.VideoModel,
			VideoResolutionOverride: cfg.Production.Volcengine.VideoResolutionOverride,
			AudioModel:              cfg.Production.Volcengine.AudioModel,
		},
	})
	sandboxJobService := sandbox.NewJobService(sandboxManager, sandboxClient, queries, storageService)
	providerRegistry.Register("internal_ffmpeg", production.NewInternalFFmpegProvider(sandboxJobService))
	providerRegistry.Register("internal_template_video", production.NewTemplateVideoProvider(sandboxJobService))
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
				APIKey:                  cfg.Production.Volcengine.APIKey,
				AudioAPIKey:             cfg.Production.Volcengine.AudioAPIKey,
				BaseURL:                 cfg.Production.Volcengine.BaseURL,
				AudioBaseURL:            cfg.Production.Volcengine.AudioBaseURL,
				Region:                  cfg.Production.Volcengine.Region,
				TextModel:               cfg.Production.Volcengine.TextModel,
				ImageModel:              cfg.Production.Volcengine.ImageModel,
				VideoModel:              cfg.Production.Volcengine.VideoModel,
				VideoResolutionOverride: cfg.Production.Volcengine.VideoResolutionOverride,
				AudioModel:              cfg.Production.Volcengine.AudioModel,
			},
			http.DefaultClient,
			time.Duration(cfg.Production.ProviderPollIntervalSeconds)*time.Second,
			time.Duration(cfg.Production.ProviderMaxPollSeconds)*time.Second,
			legacyProductionRuntime,
			inputResolver,
		)
	}
	productionRuntime = production.NewTracingRuntime(productionRuntime, agentTracing.Tracer)
	producerEnqueuer := &agentProducerTaskEnqueuer{}
	productionBroadcaster := api.NewProductionBroadcaster(canvasHub, queries, storageService)
	productionBroadcaster.SetAgentPreviewEventSink(agentPreviewEventSink{runtime: agentRuntime, broadcaster: agentBroadcaster, producerEnqueuer: producerEnqueuer})
	productionRunner := production.NewProductionRunner(
		productionService,
		productionRuntime,
		cfg.Production.WorkerConcurrency,
		productionBroadcaster,
	)
	productionRunner.SetTracer(agentTracing.Tracer)
	productionService.SetRunner(productionRunner)
	productionRunner.Start(ctx)
	agentCanvasBroadcaster := api.NewAgentCanvasNodeBroadcaster(canvasHub, queries, storageService)
	workerExecutor := agentworker.NewExecutor(agentworker.ExecutorConfig{
		Runtime:          agentRuntime,
		Store:            queries,
		Production:       productionService,
		Broadcaster:      agentCanvasBroadcaster,
		AgentBroadcaster: agentBroadcaster,
		ProducerEnqueuer: producerEnqueuer,
		Tracer:           agentTracing.Tracer,
	})
	workerEnqueuer := agentWorkerTaskEnqueuer{executor: workerExecutor}
	renderPlanSubmitter := agenttools.NewRenderPlanSubmitter(queries, agentRuntime, workerEnqueuer)
	skillRegistry := agentskills.DefaultRegistry()
	contextCompactionStore := agentcontextcompact.NewSQLStore(queries)
	contextDetailWriter := agentcontextcompact.NewSandboxDetailFileWriter(
		contextSandboxEnsurer{manager: sandboxManager},
		contextSandboxFileClient{client: sandboxClient},
	)
	contextCompactor := agentcontextcompact.NewMiddleware(agentcontextcompact.MiddlewareConfig{
		Config:         cfg.Agent.ContextCompaction,
		Store:          contextCompactionStore,
		FileWriter:     contextDetailWriter,
		FullSummarizer: contextFullSummarizerForConfig(cfg),
	})
	newReadFileTool := func() agenttools.NativeTool {
		return agenttools.NewReadFileNativeTool(sandboxManager, sandboxClient)
	}
	newEditFileTool := func() agenttools.NativeTool {
		return agenttools.NewEditFileNativeTool(sandboxManager, sandboxClient)
	}
	newSearchAgentHistoryTool := func() agenttools.NativeTool {
		return agenttools.NewSearchAgentHistoryNativeTool(contextCompactionStore, cfg.Agent.ContextCompaction)
	}
	composerNativeToolRegistry := mustNativeRegistry(
		agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleComposer),
		agenttools.NewLoadAgentSkillResourceNativeTool(skillRegistry, agentskills.RoleComposer),
		newReadFileTool(),
		newEditFileTool(),
		newSearchAgentHistoryTool(),
		agenttools.NewGetCompositionContextNativeTool(agentcomposer.NewToolContextProvider(queries)),
		agenttools.NewStageMediaInputsNativeTool(sandboxJobService),
		agenttools.NewProbeMediaNativeTool(sandboxJobService),
		agenttools.NewCreateTimelinePlanNativeTool(queries),
		agenttools.NewUpdateTimelinePlanStatusNativeTool(queries),
		agenttools.NewRenderTimelineTemplateNativeTool(agenttools.NewSandboxTimelineTemplateRenderer(sandboxJobService)),
		agenttools.NewRunFFmpegCommandNativeTool(sandboxJobService),
		agenttools.NewSubmitCompositionArtifactNativeTool(productionService, queries).WithOutputUploader(sandboxJobService),
	)
	composerGraph, err := agentcomposer.NewGraph(agentcomposer.GraphConfig{
		Loader:             agentcomposer.NewStoreContextLoader(queries),
		Runtime:            agentRuntime,
		Store:              queries,
		Production:         productionService,
		ToolResponder:      composerResponderForConfig(cfg, contextCompactor),
		NativeToolRegistry: composerNativeToolRegistry,
		Broadcaster:        agentCanvasBroadcaster,
		CheckPointStore:    agentEinoCheckpointStore,
		CompileCallbacks:   []compose.GraphCompileCallback{agentGraphInfoRegistry.CompileCallback()},
	})
	if err != nil {
		slog.Error("failed to create composer graph", "error", err)
		os.Exit(1)
	}
	composerExecutor := agentcomposer.NewExecutor(agentcomposer.ExecutorConfig{
		Runtime:          agentRuntime,
		Graph:            composerGraph,
		Broadcaster:      agentBroadcaster,
		ProducerEnqueuer: producerEnqueuer,
		TraceCallbacks:   agentTracing.Callbacks,
	})
	composerEnqueuer := agentComposerTaskEnqueuer{executor: composerExecutor}
	craftsmanGraph, err := agentcraftsman.NewGraph(agentcraftsman.GraphConfig{
		Loader: agentcraftsman.ContextLoader{
			Store:   queries,
			Runtime: agentRuntime,
		},
		ToolResponder: craftsmanResponderForConfig(cfg, contextCompactor),
		NativeToolRegistry: mustNativeRegistry(
			agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleCraftsman),
			agenttools.NewLoadAgentSkillResourceNativeTool(skillRegistry, agentskills.RoleCraftsman),
			newReadFileTool(),
			newEditFileTool(),
			newSearchAgentHistoryTool(),
			agenttools.NewReadProjectMemoryNativeTool(creativeStateService),
			agenttools.NewUpsertRenderPlanNativeTool(renderPlanService, renderPlanSubmitter).WithReferenceStore(queries),
		),
		CheckPointStore:  agentEinoCheckpointStore,
		CompileCallbacks: []compose.GraphCompileCallback{agentGraphInfoRegistry.CompileCallback()},
	})
	if err != nil {
		slog.Error("failed to create craftsman graph", "error", err)
		os.Exit(1)
	}
	craftsmanExecutor := agentcraftsman.NewExecutor(agentcraftsman.ExecutorConfig{
		Runtime:          agentRuntime,
		Graph:            craftsmanGraph,
		Broadcaster:      agentBroadcaster,
		ProducerEnqueuer: producerEnqueuer,
		TraceCallbacks:   agentTracing.Callbacks,
	})
	craftsmanEnqueuer := agentCraftsmanTaskEnqueuer{executor: craftsmanExecutor}
	reviewerNativeToolRegistry, err := agenttools.NewNativeRegistry(
		agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleReviewer),
		agenttools.NewLoadAgentSkillResourceNativeTool(skillRegistry, agentskills.RoleReviewer),
		newReadFileTool(),
		newEditFileTool(),
		newSearchAgentHistoryTool(),
		agenttools.NewReadProjectContextNativeTool(creativeStateService),
		agenttools.NewReadProjectMemoryNativeTool(creativeStateService),
		agenttools.NewSubmitReviewResultNativeTool(queries),
	)
	if err != nil {
		slog.Error("failed to create reviewer native tool registry", "error", err)
		os.Exit(1)
	}
	reviewerGraph, err := agentreviewer.NewGraph(agentreviewer.GraphConfig{
		Loader: agentreviewer.ContextLoader{
			Store:       queries,
			Runtime:     agentRuntime,
			ImageReader: storageService,
			PSSBuilder:  producerPSSBuilder,
		},
		ToolResponder:      reviewerResponderForConfig(cfg, contextCompactor),
		NativeToolRegistry: reviewerNativeToolRegistry,
		CheckPointStore:    agentEinoCheckpointStore,
		CompileCallbacks:   []compose.GraphCompileCallback{agentGraphInfoRegistry.CompileCallback()},
	})
	if err != nil {
		slog.Error("failed to create reviewer graph", "error", err)
		os.Exit(1)
	}
	reviewerExecutor := agentreviewer.NewExecutor(agentreviewer.ExecutorConfig{
		Runtime:          agentRuntime,
		Graph:            reviewerGraph,
		Broadcaster:      agentBroadcaster,
		ProducerEnqueuer: producerEnqueuer,
		TraceCallbacks:   agentTracing.Callbacks,
	})
	reviewerEnqueuer := agentReviewerTaskEnqueuer{executor: reviewerExecutor}
	producerNativeToolRegistry, err := agenttools.NewNativeRegistry(
		agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleProducer),
		agenttools.NewLoadAgentSkillResourceNativeTool(skillRegistry, agentskills.RoleProducer),
		newReadFileTool(),
		newEditFileTool(),
		newSearchAgentHistoryTool(),
		agenttools.NewReadProjectContextNativeTool(creativeStateService, producerPSSBuilder),
		agenttools.NewAnalyzeReferenceVideoNativeTool(referenceVideoAnalysisService, agenttools.NewAgentObjectRefResolver(queries)),
		agenttools.NewUpsertProjectBriefNativeTool(creativeStateService),
		agenttools.NewUpdateProjectMemoryNativeTool(creativeStateService),
		agenttools.NewUpsertKeyElementsNativeTool(creativeStateService),
		agenttools.NewUpsertStoryboardNativeTool(creativeStateService),
		agenttools.NewUpsertAudioPlanNativeTool(audioPlanService),
		agenttools.NewDispatchCraftsmanNativeTool(queries, agentRuntime, craftsmanEnqueuer),
		agenttools.NewDispatchComposerNativeTool(agentRuntime, composerEnqueuer, queries),
		agenttools.NewDecideRenderPlanNativeTool(queries, agentRuntime, workerEnqueuer),
		agenttools.NewDispatchReviewerNativeTool(queries, agentRuntime, reviewerEnqueuer),
		agenttools.NewRequestUserDecisionNativeTool(agenthitl.NewToolDecisionRequester(hitlService)),
	)
	if err != nil {
		slog.Error("failed to create producer native tool registry", "error", err)
		os.Exit(1)
	}
	producerGraph, err := agentproducer.NewGraph(agentproducer.GraphConfig{
		Loader: agentproducer.RuntimeContextLoader{
			Runtime:        agentRuntime,
			Queries:        queries,
			ImageReader:    storageService,
			Facts:          agentproducer.NewPSSFactsProvider(queries),
			ModelSelection: agentModelSelection,
		},
		Responder:          producerResponderForConfig(cfg, contextCompactor),
		NativeToolRegistry: producerNativeToolRegistry,
		SignalRuntime:      agentRuntime,
		CheckPointStore:    agentEinoCheckpointStore,
		CompileCallbacks:   []compose.GraphCompileCallback{agentGraphInfoRegistry.CompileCallback()},
	})
	if err != nil {
		slog.Error("failed to create producer graph", "error", err)
		os.Exit(1)
	}
	producerExecutor := agentproducer.NewExecutor(agentproducer.ExecutorConfig{
		Runtime:          agentRuntime,
		Graph:            producerGraph,
		Broadcaster:      agentBroadcaster,
		MaxToolCalls:     cfg.Agent.ProducerMaxToolCalls,
		ToolTimeout:      time.Duration(cfg.Agent.ToolTimeoutSeconds) * time.Second,
		TraceCallbacks:   agentTracing.Callbacks,
		ProducerEnqueuer: producerEnqueuer,
	})
	producerEnqueuer.executor = producerExecutor
	agentHandler := api.NewAgentHandler(queries, agentRuntime, agentHub, producerExecutor)
	agentHandler.SetAttachmentStorage(storageService)
	agentHandler.SetCanvasHub(canvasHub)
	agentHandler.SetModelSelectionService(agentModelSelection)
	agentHandler.SetHITLService(hitlService)
	// Disabled during Agent architecture development: local restarts often leave
	// large batches of test tasks queued, and auto-recovering them makes each
	// restart expensive and noisy.
	// go func() {
	// 	tasks, err := agentRuntime.ListQueuedProducerTasksAcrossWorkspaces(context.Background(), 1000)
	// 	if err != nil {
	// 		slog.Warn("skipping queued producer recovery", "error", err)
	// 		return
	// 	}
	// 	for _, task := range tasks {
	// 		if err := producerExecutor.RunTask(context.Background(), agentproducer.RunTaskInput{
	// 			WorkspaceID: task.WorkspaceID,
	// 			ThreadID:    task.ThreadID,
	// 			TaskID:      task.ID,
	// 		}); err != nil {
	// 			slog.Warn("failed to recover queued producer task", "task_id", task.ID, "error", err)
	// 		}
	// 	}
	// }()
	// go recoverQueuedCraftsmanTasks(craftsmanExecutor, agentRuntime)
	// go recoverQueuedWorkerTasks(workerExecutor, agentRuntime)
	// go recoverQueuedReviewerTasks(reviewerExecutor, agentRuntime)
	// go recoverQueuedComposerTasks(composerExecutor, agentRuntime)
	runHandler := api.NewRunHandler(productionService, queries, storageService)
	modelHandler := api.NewModelHandler(queries)
	referencePackHandler := api.NewReferencePackHandler(pgPool, queries, productionService)
	edgeHandler := api.NewEdgeHandler(pgPool, queries, canvasHub)
	groupHandler := api.NewGroupHandler(pgPool, queries, canvasHub)
	uploadHandler := api.NewUploadHandler(queries, storageService)
	storageHandler := api.NewStorageHandler(queries, storageService)
	canvasWSHandler := api.NewCanvasWSHandler(queries, canvasHub, cfg.JWT.Secret)
	agentWSHandler := api.NewAgentWSHandler(queries, agentHub, cfg.JWT.Secret)
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
	h.GET("/api/agent/workspaces/:workspaceID/thread", authMiddleware, agentHandler.GetThread)
	h.GET("/api/agent/workspaces/:workspaceID/threads", authMiddleware, agentHandler.ListThreads)
	h.GET("/api/agent/workspaces/:workspaceID/threads/:threadID/messages", authMiddleware, agentHandler.ListThreadMessages)
	h.GET("/api/agent/workspaces/:workspaceID/messages", authMiddleware, agentHandler.ListMessages)
	h.GET("/api/agent/workspaces/:workspaceID/tasks", authMiddleware, agentHandler.ListActiveTasks)
	h.GET("/api/agent/workspaces/:workspaceID/production-overview", authMiddleware, agentHandler.GetProductionOverview)
	h.GET("/api/agent/workspaces/:workspaceID/canvas/workbench", authMiddleware, agentHandler.GetCanvasWorkbench)
	h.GET("/api/agent/workspaces/:workspaceID/canvas/details", authMiddleware, agentHandler.GetCanvasDetail)
	h.PUT("/api/agent/workspaces/:workspaceID/canvas/layout", authMiddleware, agentHandler.PutCanvasLayout)
	h.GET("/api/agent/workspaces/:workspaceID/model-selection", authMiddleware, agentHandler.GetModelSelection)
	h.PUT("/api/agent/workspaces/:workspaceID/model-selection", authMiddleware, agentHandler.PutModelSelection)
	h.POST("/api/agent/workspaces/:workspaceID/attachments", authMiddleware, agentHandler.PostAttachment)
	h.POST("/api/agent/workspaces/:workspaceID/decisions/:eventID/respond", authMiddleware, agentHandler.PostDecision)
	h.POST("/api/agent/workspaces/:workspaceID/messages", authMiddleware, agentHandler.PostMessage)

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
	h.GET("/ws/agent", agentWSHandler.Agent)

	slog.Info("server starting", "port", cfg.Server.Port)
	h.Spin()
}

func validateRealMediaE2EConfig(cfg *config.Config) error {
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA")) != "1" {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires loaded config")
	}
	if strings.TrimSpace(cfg.Production.ProviderMode) != "real" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires production.provider_mode=real")
	}
	if strings.TrimSpace(cfg.Production.DefaultProvider) != "volcengine" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires production.default_provider=volcengine")
	}
	if strings.TrimSpace(cfg.Production.Volcengine.APIKey) == "" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY")
	}
	if strings.TrimSpace(cfg.Production.Volcengine.ImageModel) == "" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL")
	}
	if strings.TrimSpace(cfg.Production.Volcengine.AudioAPIKey) == "" && strings.TrimSpace(cfg.Production.Volcengine.APIKey) == "" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_API_KEY or CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY")
	}
	if strings.TrimSpace(cfg.Production.Volcengine.AudioModel) == "" {
		return fmt.Errorf("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 requires CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL")
	}
	return nil
}

type agentPreviewEventSink struct {
	runtime          *agentruntime.Service
	broadcaster      *api.AgentBroadcaster
	producerEnqueuer *agentProducerTaskEnqueuer
}

func (s agentPreviewEventSink) CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	if s.runtime == nil {
		return db.AgentEvent{}, nil
	}
	return s.runtime.CreateEvent(ctx, params)
}

func (s agentPreviewEventSink) BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	if s.broadcaster == nil || !event.ID.Valid {
		return
	}
	s.broadcaster.BroadcastAgentEvent(workspaceID, event)
}

func (s agentPreviewEventSink) GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if s.runtime == nil {
		return db.AgentThread{}, nil
	}
	return s.runtime.GetOrCreateProducerThread(ctx, workspaceID)
}

func (s agentPreviewEventSink) CreateProducerPendingSignal(ctx context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	if s.runtime == nil {
		return db.ProducerPendingSignal{}, nil
	}
	return s.runtime.CreateProducerPendingSignal(ctx, params)
}

func (s agentPreviewEventSink) ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error) {
	if s.runtime == nil {
		return nil, nil
	}
	return s.runtime.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
}

func (s agentPreviewEventSink) CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	if s.runtime == nil {
		return db.AgentTask{}, nil
	}
	return s.runtime.CreateTask(ctx, params)
}

func (s agentPreviewEventSink) EnqueueProducerTask(ctx context.Context, task db.AgentTask) {
	if s.producerEnqueuer == nil {
		return
	}
	s.producerEnqueuer.EnqueueProducerTask(ctx, task)
}

type agentCraftsmanTaskEnqueuer struct {
	executor *agentcraftsman.Executor
}

type agentComposerTaskEnqueuer struct {
	executor *agentcomposer.Executor
}

type agentProducerTaskEnqueuer struct {
	executor *agentproducer.Executor
}

func (e *agentProducerTaskEnqueuer) EnqueueProducerTask(ctx context.Context, task db.AgentTask) {
	if e == nil || e.executor == nil {
		return
	}
	runCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.executor.RunTask(runCtx, agentproducer.RunTaskInput{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
			TaskType:    task.TaskType,
		}); err != nil {
			slog.Warn("failed to run producer task", "task_id", task.ID, "error", err)
		}
	}()
}

func (e agentCraftsmanTaskEnqueuer) EnqueueCraftsmanTask(ctx context.Context, task db.AgentTask) {
	if e.executor == nil {
		return
	}
	runCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.executor.RunTask(runCtx, agentcraftsman.RunTaskInput{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
			ScopeType:   task.ScopeType,
			ScopeID:     task.ScopeID,
			ShotID:      shotIDForCraftsmanTask(task),
			Input:       task.Input,
		}); err != nil {
			slog.Warn("failed to run craftsman task", "task_id", task.ID, "error", err)
		}
	}()
}

func (e agentComposerTaskEnqueuer) EnqueueComposerTask(ctx context.Context, task db.AgentTask) {
	if e.executor == nil {
		return
	}
	runCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.executor.RunTask(runCtx, agentcomposer.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to run composer task", "task_id", task.ID, "error", err)
		}
	}()
}

type agentWorkerTaskEnqueuer struct {
	executor *agentworker.Executor
}

func (e agentWorkerTaskEnqueuer) EnqueueWorkerTask(ctx context.Context, task db.AgentTask) {
	if e.executor == nil {
		return
	}
	runCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.executor.RunTask(runCtx, agentworker.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to run worker task", "task_id", task.ID, "error", err)
		}
	}()
}

type agentReviewerTaskEnqueuer struct {
	executor *agentreviewer.Executor
}

func (e agentReviewerTaskEnqueuer) EnqueueReviewerTask(ctx context.Context, task db.AgentTask) {
	if e.executor == nil {
		return
	}
	runCtx := context.WithoutCancel(ctx)
	go func() {
		if err := e.executor.RunTask(runCtx, agentreviewer.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to run reviewer task", "task_id", task.ID, "error", err)
		}
	}()
}

func shotIDForCraftsmanTask(task db.AgentTask) pgtype.UUID {
	if task.ScopeType == "shot" {
		return task.ScopeID
	}
	return pgtype.UUID{}
}

/*
func recoverQueuedCraftsmanTasks(executor *agentcraftsman.Executor, runtime *agentruntime.Service) {
	if executor == nil || runtime == nil {
		return
	}
	tasks, err := runtime.ListQueuedCraftsmanTasksAcrossWorkspaces(context.Background(), 1000)
	if err != nil {
		slog.Warn("skipping queued craftsman recovery", "error", err)
		return
	}
	for _, task := range tasks {
		if err := executor.RunTask(context.Background(), agentcraftsman.RunTaskInput{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
			ScopeType:   task.ScopeType,
			ScopeID:     task.ScopeID,
			ShotID:      shotIDForCraftsmanTask(task),
			Input:       task.Input,
		}); err != nil {
			slog.Warn("failed to recover queued craftsman task", "task_id", task.ID, "error", err)
		}
	}
}

func recoverQueuedWorkerTasks(executor *agentworker.Executor, runtime *agentruntime.Service) {
	if executor == nil || runtime == nil {
		return
	}
	tasks, err := runtime.ListQueuedWorkerTasksAcrossWorkspaces(context.Background(), 1000)
	if err != nil {
		slog.Warn("skipping queued worker recovery", "error", err)
		return
	}
	for _, task := range tasks {
		if err := executor.RunTask(context.Background(), agentworker.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to recover queued worker task", "task_id", task.ID, "error", err)
		}
	}
}

func recoverQueuedComposerTasks(executor *agentcomposer.Executor, runtime *agentruntime.Service) {
	if executor == nil || runtime == nil {
		return
	}
	tasks, err := runtime.ListQueuedComposerTasksAcrossWorkspaces(context.Background(), 1000)
	if err != nil {
		slog.Warn("skipping queued composer recovery", "error", err)
		return
	}
	for _, task := range tasks {
		if err := executor.RunTask(context.Background(), agentcomposer.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to recover queued composer task", "task_id", task.ID, "error", err)
		}
	}
}

func recoverQueuedReviewerTasks(executor *agentreviewer.Executor, runtime *agentruntime.Service) {
	if executor == nil || runtime == nil {
		return
	}
	tasks, err := runtime.ListQueuedReviewerTasksAcrossWorkspaces(context.Background(), 1000)
	if err != nil {
		slog.Warn("skipping queued reviewer recovery", "error", err)
		return
	}
	for _, task := range tasks {
		if err := executor.RunTask(context.Background(), agentreviewer.RunTaskInput{Task: task}); err != nil {
			slog.Warn("failed to recover queued reviewer task", "task_id", task.ID, "error", err)
		}
	}
}
*/

func mustNativeRegistry(tools ...agenttools.NativeTool) *agenttools.NativeRegistry {
	registry, err := agenttools.NewNativeRegistry(tools...)
	if err != nil {
		slog.Error("failed to create native agent tool registry", "error", err)
		os.Exit(1)
	}
	return registry
}

const (
	producerModelMaxTokens  = 4096
	craftsmanModelMaxTokens = 8192
	reviewerModelMaxTokens  = 4096
	composerModelMaxTokens  = 4096
	summaryModelMaxTokens   = 4096
)

func contextFullSummarizerForConfig(cfg *config.Config) agentcontextcompact.FullSummarizer {
	if cfg.Production.ProviderMode != "real" ||
		strings.TrimSpace(cfg.Production.Volcengine.APIKey) == "" ||
		strings.TrimSpace(cfg.Production.Volcengine.TextModel) == "" {
		return agentcontextcompact.StaticFullSummarizer{ModelID: "static-fallback"}
	}
	return agentcontextcompact.NewVolcengineFullSummarizer(agentcontextcompact.VolcengineFullSummarizerConfig{
		APIKey:      cfg.Production.Volcengine.APIKey,
		BaseURL:     cfg.Production.Volcengine.BaseURL,
		Region:      cfg.Production.Volcengine.Region,
		Model:       cfg.Production.Volcengine.TextModel,
		MaxTokens:   summaryModelMaxTokens,
		Temperature: 0.1,
	})
}

func craftsmanResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentcraftsman.ToolCallingResponder {
	craftsmanFixture := strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_CRAFTSMAN_FIXTURE"))
	if craftsmanFixture == "template_only_video" {
		slog.Warn("using template-only video E2E craftsman fixture responder")
		return e2eTemplateOnlyVideoCraftsmanResponder{}
	}
	if craftsmanFixture == "m2_render_plan" || craftsmanFixture == "m3_reviewer_gate" {
		slog.Warn("using M2 render plan E2E craftsman fixture responder")
		return e2eM2RenderPlanCraftsmanResponder{}
	}
	return agentcraftsman.NewVolcengineModelResponder(agentcraftsman.VolcengineModelResponderConfig{
		APIKey:           cfg.Production.Volcengine.APIKey,
		BaseURL:          cfg.Production.Volcengine.BaseURL,
		Region:           cfg.Production.Volcengine.Region,
		Model:            cfg.Production.Volcengine.TextModel,
		MaxTokens:        craftsmanModelMaxTokens,
		Temperature:      0.2,
		ContextCompactor: contextCompactor,
	})
}

func producerResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentproducer.Responder {
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_PRODUCER_FIXTURE")) == "template_only_video" {
		slog.Warn("using template-only video E2E producer fixture responder")
		return e2eTemplateOnlyVideoProducerResponder{}
	}
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_PRODUCER_FIXTURE")) == "m3_reviewer_gate" {
		slog.Warn("using M3 reviewer gate E2E producer fixture responder")
		return e2eM3ReviewerGateProducerResponder{}
	}
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_PRODUCER_FIXTURE")) == "m2_render_plan" {
		slog.Warn("using M2 render plan E2E producer fixture responder")
		return e2eM2RenderPlanProducerResponder{}
	}
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_PRODUCER_FIXTURE")) == "m1_creative_state" {
		slog.Warn("using M1 creative state E2E producer fixture responder")
		return e2eM1CreativeStateResponder{}
	}
	if cfg.Production.ProviderMode != "real" ||
		strings.TrimSpace(cfg.Production.Volcengine.APIKey) == "" {
		slog.Warn(
			"using deterministic producer responder",
			"provider_mode", cfg.Production.ProviderMode,
			"has_volcengine_api_key",
			strings.TrimSpace(cfg.Production.Volcengine.APIKey) != "",
		)
		return agentproducer.DeterministicResponder{}
	}
	return agentproducer.NewVolcengineModelResponder(agentproducer.VolcengineModelResponderConfig{
		APIKey:           cfg.Production.Volcengine.APIKey,
		BaseURL:          cfg.Production.Volcengine.BaseURL,
		Region:           cfg.Production.Volcengine.Region,
		Model:            cfg.Production.Volcengine.TextModel,
		MaxTokens:        producerModelMaxTokens,
		Temperature:      0.3,
		ContextCompactor: contextCompactor,
	})
}

func reviewerResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentreviewer.ToolResponder {
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_REVIEWER_FIXTURE")) == "m3_reviewer_gate" {
		slog.Warn("using M3 reviewer gate E2E reviewer fixture responder")
		return e2eM3ReviewerGateResponder{}
	}
	return agentreviewer.NewVolcengineModelResponder(agentreviewer.VolcengineModelResponderConfig{
		APIKey:           cfg.Production.Volcengine.APIKey,
		BaseURL:          cfg.Production.Volcengine.BaseURL,
		Region:           cfg.Production.Volcengine.Region,
		Model:            cfg.Production.Volcengine.TextModel,
		MaxTokens:        reviewerModelMaxTokens,
		Temperature:      0.1,
		ContextCompactor: contextCompactor,
	})
}

func composerResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentcomposer.ToolResponder {
	if strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_COMPOSER_FIXTURE")) == "template_only_video" {
		slog.Warn("using template-only video E2E composer fixture responder")
		return e2eTemplateOnlyVideoComposerResponder{}
	}
	if cfg.Production.ProviderMode != "real" ||
		strings.TrimSpace(cfg.Production.Volcengine.APIKey) == "" {
		slog.Warn(
			"using deterministic composer responder",
			"provider_mode", cfg.Production.ProviderMode,
			"has_volcengine_api_key",
			strings.TrimSpace(cfg.Production.Volcengine.APIKey) != "",
		)
		return agentcomposer.NewDeterministicResponder()
	}
	return agentcomposer.NewVolcengineModelResponder(agentcomposer.VolcengineModelResponderConfig{
		APIKey:           cfg.Production.Volcengine.APIKey,
		BaseURL:          cfg.Production.Volcengine.BaseURL,
		Region:           cfg.Production.Volcengine.Region,
		Model:            cfg.Production.Volcengine.TextModel,
		MaxTokens:        composerModelMaxTokens,
		Temperature:      0.2,
		ContextCompactor: contextCompactor,
	})
}

type contextSandboxEnsurer struct {
	manager *sandbox.Manager
}

func (e contextSandboxEnsurer) EnsureContextSandbox(ctx context.Context, workspaceID pgtype.UUID) (agentcontextcompact.ContextSandbox, error) {
	if e.manager == nil {
		return agentcontextcompact.ContextSandbox{}, fmt.Errorf("sandbox manager is not configured")
	}
	workspaceSandbox, err := e.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		return agentcontextcompact.ContextSandbox{}, err
	}
	return agentcontextcompact.ContextSandbox{
		SandboxID:  workspaceSandbox.SandboxID,
		VolumeName: workspaceSandbox.VolumeName,
	}, nil
}

type contextSandboxFileClient struct {
	client sandbox.Client
}

func (c contextSandboxFileClient) Exec(ctx context.Context, sandboxID string, command string, cwd string, timeoutSeconds int) error {
	if c.client == nil {
		return fmt.Errorf("sandbox client is not configured")
	}
	result, err := c.client.Exec(ctx, sandboxID, sandbox.ExecRequest{
		Command:        command,
		Cwd:            cwd,
		TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("sandbox command failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (c contextSandboxFileClient) Upload(ctx context.Context, sandboxID string, path string, reader io.Reader) error {
	if c.client == nil {
		return fmt.Errorf("sandbox client is not configured")
	}
	return c.client.Upload(ctx, sandboxID, path, reader)
}

func (c contextSandboxFileClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	if c.client == nil {
		return nil, fmt.Errorf("sandbox client is not configured")
	}
	reader, _, err := c.client.Download(ctx, sandboxID, path)
	return reader, err
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
