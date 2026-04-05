package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationws "github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type apiDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
	Queue    *jobs.AsynqQueue
}

func main() {
	deps, err := bootstrapAPI(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeAPI(deps)

	log.Printf("api listening on %s", deps.Config.HTTPAddr)

	jobService := jobs.NewService(jobs.NewRepository(deps.Postgres), deps.Queue)
	registrationService := registration.NewService(jobService)
	batchService := registration.NewBatchService(jobService)
	availableServices := registration.NewAvailableServicesService(
		registration.NewAvailableServicesPostgresRepository(deps.Postgres),
	)
	statsService := registration.NewStatsService(
		registration.NewStatsPostgresRepository(deps.Postgres),
	)
	outlookService := registration.NewOutlookService(
		registration.NewOutlookPostgresRepository(deps.Postgres),
		batchService,
	)
	accountsRepository := accounts.NewPostgresRepository(deps.Postgres)
	accountsService := accounts.NewService(accountsRepository)
	settingsRepository := settings.NewPostgresRepository(deps.Postgres)
	settingsService := settings.NewService(settings.ServiceDependencies{
		Repository:    settingsRepository,
		DatabaseAdmin: settings.NewPostgresDatabaseAdmin(deps.Postgres, deps.Config.DatabaseURL, ""),
	})
	emailServicesService := emailservices.NewService(emailservices.NewPostgresRepository(deps.Postgres), nil)
	uploaderService := uploader.NewService(uploader.NewPostgresConfigRepository(deps.Postgres))
	logsService := logs.NewService(logs.NewPostgresRepository(deps.Postgres))
	taskSocketHandler := registrationws.NewHandler(jobService)
	batchSocketHandler := registrationws.NewBatchHandler(batchService)

	if err := http.ListenAndServe(
		deps.Config.HTTPAddr,
		internalhttp.NewRouter(jobService, registrationService, batchService, availableServices, statsService, outlookService, accountsService, settingsService, emailServicesService, uploaderService, logsService, taskSocketHandler, batchSocketHandler),
	); err != nil {
		log.Fatal(err)
	}
}

func bootstrapAPI(parent context.Context) (apiDependencies, error) {
	cfg, err := config.Load()
	if err != nil {
		return apiDependencies{}, err
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	pool, err := postgresplatform.OpenPool(ctx, cfg)
	if err != nil {
		return apiDependencies{}, err
	}

	redisClient, err := redisplatform.NewClient(ctx, cfg)
	if err != nil {
		pool.Close()
		return apiDependencies{}, err
	}

	queue := jobs.NewAsynqQueue(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})

	return apiDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
		Queue:    queue,
	}, nil
}

func closeAPI(deps apiDependencies) {
	if deps.Queue != nil {
		if err := deps.Queue.Close(); err != nil {
			log.Printf("close asynq queue: %v", err)
		}
	}

	if deps.Redis != nil {
		if err := deps.Redis.Close(); err != nil {
			log.Printf("close redis client: %v", err)
		}
	}

	if deps.Postgres != nil {
		deps.Postgres.Close()
	}
}
