package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/adminui"
	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	"github.com/dou-jiang/codex-console/backend-go/internal/emailservices"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/logs"
	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	registrationws "github.com/dou-jiang/codex-console/backend-go/internal/registration/ws"
	"github.com/dou-jiang/codex-console/backend-go/internal/settings"
	"github.com/dou-jiang/codex-console/backend-go/internal/team"
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
	adminUIHandler, err := adminui.NewHandler(adminui.HandlerOptions{
		BasePath: adminui.DefaultBasePath,
		Settings: apiAdminUISettingsReader{repo: settingsRepository},
	})
	if err != nil {
		log.Fatal(err)
	}
	emailServicesService := emailservices.NewService(emailservices.NewPostgresRepository(deps.Postgres), nil)
	uploaderService := newAPIUploaderService(uploader.NewPostgresConfigRepository(deps.Postgres), accountsRepository)
	logsService := logs.NewService(logs.NewPostgresRepository(deps.Postgres))
	paymentService := payment.NewService(payment.NewPostgresRepository(deps.Postgres), accountsRepository)
	teamRepository := team.NewPostgresRepository(deps.Postgres)
	teamService := team.NewService(teamRepository, nil)
	teamTaskService := team.NewTaskService(teamRepository, teamService, jobService, nil)
	taskSocketHandler := registrationws.NewHandler(jobService)
	batchSocketHandler := registrationws.NewBatchHandler(batchService)

	if err := http.ListenAndServe(
		deps.Config.HTTPAddr,
		newAPIHandler(jobService, registrationService, batchService, availableServices, statsService, outlookService, accountsService, adminUIHandler, settingsService, emailServicesService, uploaderService, logsService, paymentService, teamService, teamTaskService, taskSocketHandler, batchSocketHandler),
	); err != nil {
		log.Fatal(err)
	}
}

func newAPIHandler(jobService *jobs.Service, dependencies ...any) http.Handler {
	return internalhttp.NewRouter(jobService, dependencies...)
}

func newAPIUploaderService(repository uploader.AdminRepository, accountStore uploader.UploadAccountStore, opts ...uploader.ServiceOption) *uploader.Service {
	serviceOpts := make([]uploader.ServiceOption, 0, len(opts)+1)
	if accountStore != nil {
		serviceOpts = append(serviceOpts, uploader.WithUploadAccountStore(accountStore))
	}
	serviceOpts = append(serviceOpts, opts...)
	return uploader.NewService(repository, serviceOpts...)
}

type apiAdminUISettingsSource interface {
	GetSettings(ctx context.Context, keys []string) (map[string]settings.SettingRecord, error)
}

type apiAdminUISettingsReader struct {
	repo apiAdminUISettingsSource
}

func (r apiAdminUISettingsReader) GetSettings(ctx context.Context, keys []string) (map[string]settings.SettingRecord, error) {
	if r.repo == nil {
		return map[string]settings.SettingRecord{}, nil
	}
	items, err := r.repo.GetSettings(ctx, keys)
	if err != nil {
		return nil, err
	}
	if record, ok := items[adminui.DefaultAccessPasswordKey]; ok {
		record.Value = strings.TrimSpace(record.Value)
		items[adminui.DefaultAccessPasswordKey] = record
	}
	return items, nil
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
