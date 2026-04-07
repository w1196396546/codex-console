package main

import (
	"github.com/dou-jiang/codex-console/backend-go/internal/payment"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newAPIPaymentService(pool *pgxpool.Pool, accountsRepository payment.AccountsRepository) *payment.Service {
	return buildAPIPaymentService(payment.NewPostgresRepository(pool), accountsRepository)
}

func buildAPIPaymentService(repository payment.Repository, accountsRepository payment.AccountsRepository) *payment.Service {
	adapters := payment.NewTransitionAdapters()
	return payment.NewService(
		repository,
		accountsRepository,
		payment.WithCheckoutLinkGenerator(adapters.CheckoutLinkGenerator),
		payment.WithBillingProfileGenerator(adapters.BillingProfileGenerator),
		payment.WithBrowserOpener(adapters.BrowserOpener),
		payment.WithSessionAdapter(adapters.SessionAdapter),
		payment.WithSubscriptionChecker(adapters.SubscriptionChecker),
		payment.WithAutoBinder(adapters.AutoBinder),
	)
}
