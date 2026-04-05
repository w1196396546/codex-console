package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/accounts"
	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	"github.com/dou-jiang/codex-console/backend-go/internal/nativerunner"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/dou-jiang/codex-console/backend-go/internal/registration"
	"github.com/dou-jiang/codex-console/backend-go/internal/uploader"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type workerDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
	Queue    *jobs.AsynqQueue
	Server   *asynq.Server
	Service  *jobs.Service
}

func main() {
	deps, err := bootstrapWorker(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeWorker(deps)

	accountRepository := accounts.NewPostgresRepository(deps.Postgres)
	registrationRunner, err := newWorkerRegistrationRunner(accountRepository, deps.Redis)
	if err != nil {
		log.Fatal(err)
	}
	accountService := accounts.NewService(accountRepository)
	autoUploadDispatcher := registration.NewAutoUploadDispatcher(
		uploader.NewPostgresConfigRepository(deps.Postgres),
		nil,
	)
	registrationExecutor := newRegistrationExecutor(
		deps.Service,
		registrationRunner,
		accountService,
		autoUploadDispatcher,
		newRegistrationPreparationDependencies(deps.Postgres),
	)
	worker := jobs.NewWorkerWithIDAndExecutor(deps.Service, "worker-main", newWorkerExecutor(registrationExecutor))
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TypeGenericJob, worker.HandleTask)

	log.Print("worker bootstrap started")
	if err := deps.Server.Run(mux); err != nil {
		log.Fatal(err)
	}
}

func bootstrapWorker(parent context.Context) (workerDependencies, error) {
	cfg, err := config.Load()
	if err != nil {
		return workerDependencies{}, err
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	pool, err := postgresplatform.OpenPool(ctx, cfg)
	if err != nil {
		return workerDependencies{}, err
	}

	redisClient, err := redisplatform.NewClient(ctx, cfg)
	if err != nil {
		pool.Close()
		return workerDependencies{}, err
	}

	queue := jobs.NewAsynqQueue(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})
	service := jobs.NewService(jobs.NewRepository(pool), queue)
	server := asynq.NewServer(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	}, asynq.Config{
		Concurrency: cfg.WorkerConcurrency,
	})

	return workerDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
		Queue:    queue,
		Server:   server,
		Service:  service,
	}, nil
}

func newRegistrationExecutor(
	logs *jobs.Service,
	runner registration.Runner,
	accountService *accounts.Service,
	autoUploadDispatcher registration.AutoUploadDispatcher,
	preparationDeps registration.PreparationDependencies,
) *registration.Executor {
	return registration.NewExecutor(
		logs,
		runner,
		registration.WithPreparationDependencies(preparationDeps),
		registration.WithAccountPersistence(accountService),
		registration.WithAutoUploadDispatcher(autoUploadDispatcher),
	)
}

func newRegistrationPreparationDependencies(pool *pgxpool.Pool) registration.PreparationDependencies {
	availableServicesRepo := registration.NewAvailableServicesPostgresRepository(pool)
	return registration.PreparationDependencies{
		Settings:      availableServicesRepo,
		EmailServices: availableServicesRepo,
		Outlook:       registration.NewOutlookPostgresRepository(pool),
		Proxies:       registration.NewPostgresProxySelector(pool, availableServicesRepo),
		Reservations:  registration.NewJobPayloadOutlookReservationStore(pool),
	}
}

func newWorkerRegistrationRunner(accountRepository accounts.Repository, redisClient *redisv9.Client) (registration.Runner, error) {
	tokenCompletionCoordinator := nativerunner.NewTokenCompletionCoordinator(nativerunner.TokenCompletionCoordinatorOptions{
		Scheduler: nativerunner.NewTokenCompletionScheduler(nativerunner.DefaultTokenCompletionSchedulerPolicy()),
		Provider: nativerunner.NewStrategyTokenCompletionProvider(
			nativerunner.NewAuthPasswordTokenCompletionProvider(nil),
			nativerunner.NewAuthPasswordlessTokenCompletionProvider(nil),
		),
		RuntimeStore: newWorkerTokenCompletionRuntimeStore(accountRepository),
		LeaseStore:   newWorkerTokenCompletionLeaseStore(redisClient),
	})

	return registration.NewNativeRunner(nativerunner.NewDefault(nativerunner.DefaultOptions{
		HistoricalPasswordProvider:      newWorkerHistoricalPasswordProvider(accountRepository),
		TokenCompletionCooldownProvider: newWorkerTokenCompletionCooldownProvider(accountRepository),
		TokenCompletionAttemptProvider:  newWorkerTokenCompletionAttemptProvider(accountRepository),
		TokenCompletionCoordinator:      tokenCompletionCoordinator,
	})), nil
}

func newWorkerHistoricalPasswordProvider(accountRepository accounts.Repository) nativerunner.HistoricalPasswordProvider {
	if accountRepository == nil {
		return nativerunner.HistoricalPasswordProviderFunc(func(context.Context, nativerunner.FlowRequest, string) (string, error) {
			return "", nil
		})
	}

	return nativerunner.HistoricalPasswordProviderFunc(func(ctx context.Context, _ nativerunner.FlowRequest, email string) (string, error) {
		account, found, err := accountRepository.GetAccountByEmail(ctx, strings.TrimSpace(email))
		if err != nil {
			return "", err
		}
		if !found {
			return "", nil
		}
		return strings.TrimSpace(account.Password), nil
	})
}

func newWorkerTokenCompletionCooldownProvider(accountRepository accounts.Repository) nativerunner.TokenCompletionCooldownProvider {
	if accountRepository == nil {
		return nativerunner.TokenCompletionCooldownProviderFunc(func(context.Context, nativerunner.FlowRequest, string) (*time.Time, error) {
			return nil, nil
		})
	}

	return nativerunner.TokenCompletionCooldownProviderFunc(func(ctx context.Context, _ nativerunner.FlowRequest, email string) (*time.Time, error) {
		account, found, err := accountRepository.GetAccountByEmail(ctx, strings.TrimSpace(email))
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		runtimeState, err := nativerunner.ParseTokenCompletionRuntimeState(account.ExtraData, email)
		if err != nil {
			return nil, err
		}
		return runtimeState.CooldownUntil, nil
	})
}

func newWorkerTokenCompletionAttemptProvider(accountRepository accounts.Repository) nativerunner.TokenCompletionAttemptProvider {
	if accountRepository == nil {
		return nativerunner.TokenCompletionAttemptProviderFunc(func(context.Context, nativerunner.FlowRequest, string) ([]nativerunner.TokenCompletionAttempt, error) {
			return nil, nil
		})
	}

	return nativerunner.TokenCompletionAttemptProviderFunc(func(ctx context.Context, _ nativerunner.FlowRequest, email string) ([]nativerunner.TokenCompletionAttempt, error) {
		account, found, err := accountRepository.GetAccountByEmail(ctx, strings.TrimSpace(email))
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		runtimeState, err := nativerunner.ParseTokenCompletionRuntimeState(account.ExtraData, email)
		if err != nil {
			return nil, err
		}
		return runtimeState.Attempts, nil
	})
}

func newWorkerTokenCompletionRuntimeStore(accountRepository accounts.Repository) nativerunner.TokenCompletionRuntimeStore {
	if accountRepository == nil {
		return nil
	}

	return &workerTokenCompletionRuntimeStore{accountRepository: accountRepository}
}

func newWorkerTokenCompletionLeaseStore(redisClient *redisv9.Client) nativerunner.TokenCompletionLeaseStore {
	if redisClient == nil {
		return nil
	}
	return &workerTokenCompletionLeaseStore{
		backend: workerTokenCompletionRedisClientAdapter{client: redisClient},
	}
}

type workerTokenCompletionRuntimeStore struct {
	accountRepository accounts.Repository
}

type workerTokenCompletionLeaseStore struct {
	backend workerTokenCompletionLeaseRedisBackend
}

type workerTokenCompletionCompareAndSwapRepository interface {
	CompareAndSwapTokenCompletionRuntime(ctx context.Context, email string, currentExtraData map[string]any, nextExtraData map[string]any, defaults accounts.Account) (accounts.Account, bool, error)
}

type workerTokenCompletionLeaseRedisBackend interface {
	SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

type workerTokenCompletionRedisClientAdapter struct {
	client *redisv9.Client
}

func (a workerTokenCompletionRedisClientAdapter) SetNX(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return a.client.SetNX(ctx, key, value, expiration).Result()
}

func (a workerTokenCompletionRedisClientAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.client.Get(ctx, key).Result()
}

func (a workerTokenCompletionRedisClientAdapter) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return a.client.Eval(ctx, script, keys, args...).Result()
}

const workerTokenCompletionLeaseRenewScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`

const workerTokenCompletionLeaseReleaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

func (s *workerTokenCompletionRuntimeStore) Load(ctx context.Context, email string) (nativerunner.TokenCompletionRuntimeState, error) {
	if s == nil || s.accountRepository == nil {
		return nativerunner.TokenCompletionRuntimeState{}, nil
	}

	account, found, err := s.accountRepository.GetAccountByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return nativerunner.TokenCompletionRuntimeState{}, err
	}
	if !found {
		return nativerunner.TokenCompletionRuntimeState{}, nil
	}
	return nativerunner.ParseTokenCompletionRuntimeState(account.ExtraData, email)
}

func (s *workerTokenCompletionLeaseStore) Claim(ctx context.Context, email string, leaseToken string, ttl time.Duration) (bool, error) {
	if s == nil || s.backend == nil {
		return true, nil
	}
	return s.backend.SetNX(ctx, workerTokenCompletionLeaseKey(email), strings.TrimSpace(leaseToken), ttl)
}

func (s *workerTokenCompletionLeaseStore) Renew(ctx context.Context, email string, leaseToken string, ttl time.Duration) (bool, error) {
	if s == nil || s.backend == nil {
		return true, nil
	}
	result, err := s.backend.Eval(
		ctx,
		workerTokenCompletionLeaseRenewScript,
		[]string{workerTokenCompletionLeaseKey(email)},
		strings.TrimSpace(leaseToken),
		ttl.Milliseconds(),
	)
	if err != nil {
		return false, err
	}
	return workerTokenCompletionEvalBool(result), nil
}

func (s *workerTokenCompletionLeaseStore) Release(ctx context.Context, email string, leaseToken string) error {
	if s == nil || s.backend == nil {
		return nil
	}
	_, err := s.backend.Eval(
		ctx,
		workerTokenCompletionLeaseReleaseScript,
		[]string{workerTokenCompletionLeaseKey(email)},
		strings.TrimSpace(leaseToken),
	)
	return err
}

func (s *workerTokenCompletionLeaseStore) IsActive(ctx context.Context, email string, leaseToken string) (bool, error) {
	if s == nil || s.backend == nil {
		return true, nil
	}
	value, err := s.backend.Get(ctx, workerTokenCompletionLeaseKey(email))
	if err != nil {
		if errors.Is(err, redisv9.Nil) {
			return false, nil
		}
		return false, err
	}
	return value == strings.TrimSpace(leaseToken), nil
}

func (s *workerTokenCompletionRuntimeStore) Save(ctx context.Context, email string, state nativerunner.TokenCompletionRuntimeState) error {
	if s == nil || s.accountRepository == nil {
		return nil
	}

	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" {
		return nil
	}

	account, found, err := s.accountRepository.GetAccountByEmail(ctx, normalizedEmail)
	if err != nil {
		return err
	}

	extraData := make(map[string]any, len(account.ExtraData)+2)
	for key, value := range account.ExtraData {
		extraData[key] = value
	}
	for key, value := range nativerunner.TokenCompletionRuntimeExtraData(state) {
		extraData[key] = value
	}

	if !found {
		account = accounts.Account{
			Email:  normalizedEmail,
			Status: "token_pending",
			Source: "login",
		}
	}

	account.Email = normalizedEmail
	account.ExtraData = extraData
	if strings.TrimSpace(account.Status) == "" {
		account.Status = "token_pending"
	}
	if strings.TrimSpace(account.Source) == "" {
		account.Source = "login"
	}

	_, err = s.accountRepository.UpsertAccount(ctx, account)
	return err
}

func (s *workerTokenCompletionRuntimeStore) CompareAndSwap(ctx context.Context, email string, current nativerunner.TokenCompletionRuntimeState, next nativerunner.TokenCompletionRuntimeState) (bool, error) {
	if s == nil || s.accountRepository == nil {
		return true, nil
	}

	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" {
		return true, nil
	}

	defaults := accounts.Account{
		Email:  normalizedEmail,
		Status: "token_pending",
		Source: "login",
	}
	if casRepo, ok := s.accountRepository.(workerTokenCompletionCompareAndSwapRepository); ok {
		_, swapped, err := casRepo.CompareAndSwapTokenCompletionRuntime(
			ctx,
			normalizedEmail,
			nativerunner.TokenCompletionRuntimeExtraData(current),
			nativerunner.TokenCompletionRuntimeExtraData(next),
			defaults,
		)
		return swapped, err
	}

	loaded, err := s.Load(ctx, normalizedEmail)
	if err != nil {
		return false, err
	}
	if !workerTokenCompletionRuntimeStateEqual(loaded, current) {
		return false, nil
	}
	return true, s.Save(ctx, normalizedEmail, next)
}

func workerTokenCompletionRuntimeStateEqual(left nativerunner.TokenCompletionRuntimeState, right nativerunner.TokenCompletionRuntimeState) bool {
	leftJSON, err := json.Marshal(nativerunner.TokenCompletionRuntimeExtraData(left))
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(nativerunner.TokenCompletionRuntimeExtraData(right))
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func workerTokenCompletionLeaseKey(email string) string {
	return "codex-console:token-completion:lease:" + strings.ToLower(strings.TrimSpace(email))
}

func workerTokenCompletionEvalBool(value any) bool {
	switch typed := value.(type) {
	case int64:
		return typed != 0
	case bool:
		return typed
	case string:
		return typed == "1" || strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func newWorkerExecutor(registrationExecutor jobs.Executor) jobs.Executor {
	return jobs.ExecutorFunc(func(ctx context.Context, job jobs.Job) (map[string]any, error) {
		if registrationExecutor == nil {
			return nil, fmt.Errorf("registration executor is required")
		}
		if job.JobType != registration.JobTypeSingle {
			return nil, fmt.Errorf("unsupported worker job type: %s", job.JobType)
		}
		return registrationExecutor.Execute(ctx, job)
	})
}

func closeWorker(deps workerDependencies) {
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
