package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Shamba-Records-Limited/microvault/pkg/telemetry"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/swagger"

	_ "github.com/Shamba-Records-Limited/microvault-credit/cmd/credit/docs"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/adapters"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/handlers"
	creditnotifications "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/pkg/ratelimit"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/account"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/compliance/elliptic"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/controllers"
	"github.com/Shamba-Records-Limited/microvault/pkg/health"
	"github.com/Shamba-Records-Limited/microvault/pkg/logging"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms/providers/africastalking"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	atussd "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/providers/africastalking"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/fonbnk"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/relay"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/relay/sources"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/pin"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/accountheal"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/airtelpoller"
	compliancesvc "github.com/Shamba-Records-Limited/microvault/pkg/services/compliance"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mpesapoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/vaultwatch"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	stellarrpc "github.com/Shamba-Records-Limited/microvault/pkg/stellar/rpc"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
	"github.com/Shamba-Records-Limited/microvault/pkg/user"
	"github.com/Shamba-Records-Limited/microvault/pkg/validation"
	"github.com/Shamba-Records-Limited/microvault/pkg/webhook"
	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
)

// @title Microvault Credit API
// @version 1.0
// @description Credit management and loan processing service for microvault.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email smugane@shambarecords.com
// @license.name AGPL-3.0
// @license.url https://www.gnu.org/licenses/agpl-3.0.en.html
// @host localhost:8081
// @BasePath /
func main() {
	logger := logging.Setup()

	shutdownTracing, err := telemetry.Setup(context.Background())
	if err != nil {
		log.Fatalf("Failed to set up tracing: %v", err)
	}

	// ---- 1. Configuration ----
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// ---- 2. Database ----
	db, err := database.GetConnection("credit", &cfg.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	slog.Info("Database connected successfully")

	// ---- 3. Redis ----
	redisClient, err := cache.GetConnection("credit", &cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	slog.Info("Redis connected successfully")

	// ---- 3b. Auth services ----
	challengeStore, err := auth.NewRedisStore(redisClient, "microvault:auth")
	if err != nil {
		log.Fatalf("Failed to initialize challenge store: %v", err)
	}
	challengeService, err := auth.NewChallengeService(&cfg.Auth, &cfg.Stellar, challengeStore)
	if err != nil {
		log.Fatalf("Failed to initialize challenge service: %v", err)
	}
	jwtService := auth.NewJWTService(&cfg.Auth)
	validationService := validation.NewValidatorService()
	authController := controllers.NewAuthController(challengeService, jwtService, cfg.Stellar.AdminPublicKey, validationService)

	// ---- 4. Repositories ----
	repos, err := repository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize repositories: %v", err)
	}

	// ---- 4b. Core repositories (shared DB — users, accounts, transactions) ----
	coreRepos, err := corerepository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize core repositories: %v", err)
	}

	// A derivation index handed out twice derives one keypair for two users,
	// and the second account already exists on-chain with its master key at
	// weight 0 — unusable and unrecoverable. The sequence guarantees this only
	// while it moves forward, so re-arm it at every boot from the highest index
	// the rows still record. AccountIndexBase covers what the rows cannot: a
	// database rebuilt from scratch while the on-chain accounts persisted.
	if cfg.Stellar.AccountIndexBase <= 0 && cfg.Server.ServerEnvironment != "development" {
		log.Fatalf("STELLAR_ACCOUNT_INDEX_BASE must be set outside development: " +
			"without it a rebuilt database re-derives keypairs whose Stellar accounts already exist")
	}
	nextIndex, err := coreRepos.Account.EnsureAccountIndexIntegrity(context.Background(), cfg.Stellar.AccountIndexBase)
	if err != nil {
		log.Fatalf("Failed to floor account index sequence: %v", err)
	}
	slog.Info("account index sequence floored", slog.Int64("next_index", nextIndex))

	// ---- 5. Loan service ----
	loanSvc := loan.NewService(repos.Loan, cfg.Payments.LoanReferencePrefix)

	// ---- 6. Stellar RPC client + service ----
	rpcClient := cfg.Stellar.NewRpcClient()
	stellarSvc := stellar.NewService(
		rpcClient,
		cfg.Stellar.NetworkPassphrase,
		cfg.Stellar.TreasurySecretKey,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)

	// ---- 7. SMS + notification service ----
	smsService := sms.NewSMSService()
	atSMSAdapter := africastalking.NewAfricasTalkingSMSAdapter(
		cfg.Mobile.AfricasTalking.Username,
		cfg.Mobile.AfricasTalking.APIKey,
		cfg.Mobile.AfricasTalking.BaseURL,
		cfg.Mobile.AfricasTalking.HTTPTimeout,
	)
	smsService.RegisterProvider("africastalking", atSMSAdapter)

	// Resolve SMS provider and create the transport. The loan and account
	// notifiers are built later, once userSvc exists to back the language
	// resolver they take at construction.
	atProvider, _ := smsService.GetProvider("africastalking")
	notifier := mvnotifications.NewSMSNotifier(atProvider, cfg.Mobile.AfricasTalking.ResolveSenderID())

	// ---- 8. YellowCard adapter ----
	ycAdapter := yellowcard.NewYellowcardAdapter(
		cfg.Payments.YellowCard.PublicKey,
		cfg.Payments.YellowCard.SecretKey,
		cfg.Payments.YellowCard.BaseURL,
	)

	// ---- 9. Treasury transfer bridge ----
	// Derived here rather than at first use: both the Fonbnk off-ramp and the
	// repayment rail need it, and the two are wired far apart.
	treasuryAddr, err := cfg.Stellar.TreasuryAddress()
	if err != nil {
		log.Fatalf("Treasury address unresolved: %v", err)
	}
	treasuryTransfer := ussdadapters.NewStellarTreasuryTransfer(stellarSvc, logger)

	// ---- 10. Off-ramp adapters ----
	// Both YellowCard (mobile money) and MoneyGram (cash pickup) are
	// platform defaults — no env-gate. Each provider is built and
	// registered once; the registry drives per-request dispatch inside
	// LoanServiceAdapter, the YC adapter is passed directly to the YC-
	// specific refund poller and rate service, and the FXOrchestrator
	// cascades MG to YC for rate quoting inside the loan adapter.

	// 10a. YellowCard off-ramp adapter
	//
	// YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE: test-only knob. When set,
	// every YC payment uses this number instead of the real recipient — used
	// for hitting YC sandbox simulation phones (e.g. +2341111111111 for a
	// guaranteed-success momo transaction). Hard-disabled in production:
	// even if the env var is set, the override is dropped when
	// SERVER_ENVIRONMENT=production.
	ycTestPhoneOverride := os.Getenv("YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE")
	if ycTestPhoneOverride != "" && os.Getenv("SERVER_ENVIRONMENT") == "production" {
		logger.Error("YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE is set but SERVER_ENVIRONMENT=production — override ignored")
		ycTestPhoneOverride = ""
	}
	ycTestAddressOverride := os.Getenv("YELLOWCARD_TEST_DESTINATION_ADDRESS_OVERRIDE")
	if ycTestAddressOverride != "" && os.Getenv("SERVER_ENVIRONMENT") == "production" {
		logger.Error("YELLOWCARD_TEST_DESTINATION_ADDRESS_OVERRIDE is set but SERVER_ENVIRONMENT=production — override ignored")
		ycTestAddressOverride = ""
	}
	ycOffRamp := ussdadapters.NewYellowCardOffRampAdapter(ussdadapters.YellowCardOffRampConfig{
		Adapter:                        ycAdapter,
		Treasury:                       treasuryTransfer,
		BusinessID:                     cfg.Payments.YellowCard.BusinessID,
		BusinessName:                   cfg.Payments.YellowCard.BusinessName,
		Logger:                         logger,
		TestDestinationPhoneOverride:   ycTestPhoneOverride,
		TestDestinationAddressOverride: ycTestAddressOverride,
	})
	offRampRegistry := offramp.NewRegistry()
	if err := offRampRegistry.Register(ycOffRamp); err != nil {
		log.Fatalf("Failed to register YellowCard off-ramp: %v", err)
	}
	if err := offRampRegistry.Alias(offramp.PayoutMethodMobileMoney, offramp.ProviderYellowCard); err != nil {
		log.Fatalf("Failed to alias mobile_money → yellowcard: %v", err)
	}

	// ---- 10a. MoneyGram cash-pickup adapter ----
	// MG is the platform's default cash-pickup anchor: we always fetch the
	// anchor's TOML, validate it against the pinned signing key + network
	// passphrase, construct the SDK client, register the adapter, and alias
	// cash_pickup → moneygram. Boot fails loudly if any of that breaks.
	if err := cfg.Payments.MoneyGram.Validate(); err != nil {
		log.Fatalf("MoneyGram config invalid: %v", err)
	}
	if err := cfg.Payments.Mpesa.Validate(cfg.Server.ServerEnvironment); err != nil {
		log.Fatalf("M-Pesa config invalid: %v", err)
	}
	tomlCtx, tomlCancel := context.WithTimeout(context.Background(), 15*time.Second)
	mgTOML, err := stellaranchor.FetchTOML(tomlCtx, nil, cfg.Payments.MoneyGram.HomeDomain)
	tomlCancel()
	if err != nil {
		log.Fatalf("MoneyGram TOML fetch failed for %s: %v", cfg.Payments.MoneyGram.HomeDomain, err)
	}
	if err := mgTOML.Validate(stellaranchor.ValidateOptions{
		ExpectedNetworkPassphrase: cfg.Payments.MoneyGram.NetworkPassphrase,
		ExpectedSigningKey:        cfg.Payments.MoneyGram.ServerSigningKey,
		ExpectedUSDCIssuer:        cfg.Payments.MoneyGram.USDCIssuer,
	}); err != nil {
		log.Fatalf("MoneyGram TOML validation failed: %v", err)
	}

	// TransferServerURL override from env wins over the TOML; otherwise
	// fall back to whatever the anchor publishes.
	transferServerURL := cfg.Payments.MoneyGram.TransferServerURL
	if transferServerURL == "" {
		transferServerURL = mgTOML.TransferServerSEP24
	}

	mgCfg := moneygram.Config{
		HomeDomain:        cfg.Payments.MoneyGram.HomeDomain,
		WebAuthEndpoint:   mgTOML.WebAuthEndpoint,
		TransferServerURL: transferServerURL,
		ServerSigningKey:  cfg.Payments.MoneyGram.ServerSigningKey,
		NetworkPassphrase: cfg.Payments.MoneyGram.NetworkPassphrase,
		USDCIssuer:        cfg.Payments.MoneyGram.USDCIssuer,
		TreasurySecret:    cfg.Payments.MoneyGram.AuthSecret,
		Logger:            logger,
	}
	if cfg.Payments.MoneyGram.HasRESTCredentials() {
		mgCfg.REST = moneygram.RESTConfig{
			BaseURL:       cfg.Payments.MoneyGram.FXRateURL,
			OAuthTokenURL: cfg.Payments.MoneyGram.OAuthURL,
			ClientID:      cfg.Payments.MoneyGram.ClientID,
			ClientSecret:  cfg.Payments.MoneyGram.ClientSecret,
			Scope:         "fx_rate",
		}
	}
	mgClient, err := moneygram.New(mgCfg)
	if err != nil {
		log.Fatalf("MoneyGram client construction failed: %v", err)
	}

	// Funds wallet: SEP-24 account + USDC send source. Distinct from the auth
	// wallet in prod; both default to TREASURY_SECRET_KEY.
	mgFundsAddr, err := cfg.Payments.MoneyGram.FundsAddress()
	if err != nil {
		log.Fatalf("MoneyGram funds address derivation failed: %v", err)
	}
	mgFundsSvc := stellar.NewService(
		rpcClient,
		cfg.Stellar.NetworkPassphrase,
		cfg.Payments.MoneyGram.FundsSecret,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)
	mgFundsTransfer := ussdadapters.NewStellarTreasuryTransfer(mgFundsSvc, logger)
	// Fatal rather than ignored: the child-memo namespace is derived from this
	// address, so an empty one silently puts every SEP-24 session in a memo
	// space nothing else queries.
	mgAuthAddr, err := cfg.Payments.MoneyGram.AuthAddress()
	if err != nil {
		log.Fatalf("MoneyGram auth address derivation failed: %v", err)
	}
	logger.InfoContext(tomlCtx, "moneygram wallets resolved", "auth_address", mgAuthAddr, "funds_address", mgFundsAddr)

	mgAdapter, err := ussdadapters.NewMoneyGramOffRampAdapter(ussdadapters.MoneyGramOffRampConfig{
		Client:      mgClient,
		FundsPubkey: mgFundsAddr,
		Logger:      logger,
	})
	if err != nil {
		log.Fatalf("MoneyGram off-ramp adapter construction failed: %v", err)
	}
	if err := offRampRegistry.Register(mgAdapter); err != nil {
		log.Fatalf("Failed to register MoneyGram off-ramp: %v", err)
	}
	if err := offRampRegistry.Alias(offramp.PayoutMethodCashPickup, offramp.ProviderMoneyGram); err != nil {
		log.Fatalf("Failed to alias cash_pickup → moneygram: %v", err)
	}
	slog.InfoContext(tomlCtx, "MoneyGram cash-pickup registered", slog.String("home_domain", cfg.Payments.MoneyGram.HomeDomain), slog.Bool("has_rest_credentials", cfg.Payments.MoneyGram.HasRESTCredentials()))

	// ---- 10c. Fonbnk off-ramp and the provider relay ----
	// Fonbnk names the asset by network, and carrier codes come from its own
	// discovery endpoint. Both are fixed for this deployment's corridor.
	const fonbnkCryptoCode = "STELLAR_USDC"
	fonbnkCarrierCodes := map[string]string{"KES": "ke_safaricom"}

	// Two independent gates. Fonbnk is registered only when credentials are
	// present, and the relay only routes when
	// ENABLE_PAYMENT_PROVIDER_RELAY_SWITCH is on. With the relay off,
	// mobile_money still resolves to YellowCard exactly as before, whether or
	// not Fonbnk is wired.
	relayRegistry := relay.NewRegistry()

	ycSource, err := sources.NewYellowCardSource(sources.YellowCardSourceConfig{Client: ycAdapter})
	if err != nil {
		log.Fatalf("YellowCard rate source construction failed: %v", err)
	}
	if err := relayRegistry.Register(ycSource); err != nil {
		log.Fatalf("Failed to register the YellowCard rate source: %v", err)
	}

	if cfg.Payments.Fonbnk.ClientID != "" && cfg.Payments.Fonbnk.ClientSecret != "" {
		fonbnkClient := fonbnk.NewFonbnkAdapter(
			cfg.Payments.Fonbnk.ClientID,
			cfg.Payments.Fonbnk.ClientSecret,
			cfg.Payments.Fonbnk.BaseURL,
		)

		fonbnkOffRamp, err := ussdadapters.NewFonbnkOffRampAdapter(ussdadapters.FonbnkOffRampConfig{
			Client:             fonbnkClient,
			Treasury:           treasuryTransfer,
			CryptoCurrencyCode: fonbnkCryptoCode,
			TreasuryAddress:    treasuryAddr,
			Logger:             logger,
		})
		if err != nil {
			log.Fatalf("Fonbnk off-ramp adapter construction failed: %v", err)
		}
		if err := offRampRegistry.Register(fonbnkOffRamp); err != nil {
			log.Fatalf("Failed to register Fonbnk off-ramp: %v", err)
		}
		// A separate alias from mobile_money: the relay pins it explicitly,
		// so the unrouted default keeps going to YellowCard.
		if err := offRampRegistry.Alias(adapters.PayoutMethodFonbnkMobileMoney, offramp.ProviderFonbnk); err != nil {
			log.Fatalf("Failed to alias mobile_money_fonbnk → fonbnk: %v", err)
		}

		fonbnkSource, err := sources.NewFonbnkSource(sources.FonbnkSourceConfig{
			Client:             fonbnkClient,
			CryptoCurrencyCode: fonbnkCryptoCode,
			CarrierCodes:       fonbnkCarrierCodes,
		})
		if err != nil {
			log.Fatalf("Fonbnk rate source construction failed: %v", err)
		}
		if err := relayRegistry.Register(fonbnkSource); err != nil {
			log.Fatalf("Failed to register the Fonbnk rate source: %v", err)
		}
		slog.InfoContext(tomlCtx, "Fonbnk off-ramp registered", slog.String("base_url", cfg.Payments.Fonbnk.BaseURL))
	} else {
		slog.InfoContext(tomlCtx, "Fonbnk credentials absent — off-ramp and rate source not registered")
	}

	relayRouter, err := relay.New(relay.Config{
		Registry: relayRegistry,
		Enabled:  cfg.Payments.EnableProviderRelaySwitch,
		Default:  string(offramp.ProviderYellowCard),
		Logger:   logger,
	})
	if err != nil {
		log.Fatalf("Payment relay construction failed: %v", err)
	}
	slog.InfoContext(tomlCtx, "payment relay configured", slog.Bool("enabled", relayRouter.Enabled()), slog.Any("names", relayRegistry.Names()))

	// 10d. FX orchestrator: MG primary, YC fallback, stale cache last resort.
	ycFallback := moneygram.FallbackRateFunc(func(ctx context.Context, currency string) (float64, error) {
		rates, err := ycAdapter.GetRates(ctx, currency)
		if err != nil {
			return 0, err
		}
		// Match on Code rather than trusting position. The ?currency= filter is
		// honoured today, so rates[0] is right — but quoting a loan at another
		// market's rate is not a failure worth risking on that assumption.
		for _, r := range rates {
			if strings.EqualFold(r.Code, currency) {
				if r.Sell <= 0 {
					return 0, fmt.Errorf("yellowcard: no sell rate for %s", currency)
				}
				return r.Sell, nil
			}
		}
		return 0, fmt.Errorf("yellowcard: no rates for %s", currency)
	})
	fxOrch, err := mgClient.NewFXOrchestrator(ycFallback, moneygram.FXOrchestratorConfig{
		EntryBufferPct:         cfg.Payments.MoneyGram.FXEntryBufferPct,
		EntryBufferPctFallback: cfg.Payments.MoneyGram.FXEntryBufferPctFallback,
	})
	if err != nil {
		log.Fatalf("MoneyGram FX orchestrator init failed: %v", err)
	}
	logger.InfoContext(tomlCtx, "moneygram FX orchestrator wired",
		"primary_active", mgClient.HasFXRate(),
		"fallback_active", true,
		"entry_buffer_pct", fxOrch.EntryBufferPct(),
		"entry_buffer_pct_fallback", fxOrch.EntryBufferPctFallback(),
	)

	// ---- 10b. User + Account + Transaction services ----
	userSvc := user.NewService(coreRepos.User)
	accountSvc := account.NewService(coreRepos.Account, coreRepos.User)
	txnSvc := transaction.NewService(coreRepos.Transaction)

	// ---- 10c. UserServiceAdapter ----
	userAdapter, err := ussdadapters.NewUserServiceAdapter(
		userSvc,
		accountSvc,
		stellarSvc,
		db,
		ussdadapters.WalletConfig{
			TreasuryPrivateKey: cfg.Stellar.TreasurySecretKey,
			USDCIssuer:         cfg.Stellar.USDCIssuer,
			EnableMultiSig:     cfg.Stellar.EnableMultiSig,
			LowThreshold:       cfg.Stellar.MultiSigLowThreshold,
			MediumThreshold:    cfg.Stellar.MultiSigMediumThreshold,
			HighThreshold:      cfg.Stellar.MultiSigHighThreshold,
		},
		logger,
		nil, // AlertService — chain-status failures log-only for now
	)
	if err != nil {
		log.Fatalf("Failed to create user service adapter: %v", err)
	}

	// ---- 10c2. SMS language resolver + loan notifier ----
	// Resolve SMS language from the recipient's stored preference so every
	// notification (including background poller/job sends) is localized.
	langResolver := func(ctx context.Context, phone string) string {
		u, err := userSvc.GetByMobileNumber(ctx, phone)
		if err != nil || u == nil {
			return ""
		}
		return u.PreferredLanguage
	}

	loanNotifier, err := mvnotifications.NewSMSLoanNotifier(notifier,
		mvnotifications.WithLoanLanguageResolver(langResolver),
		mvnotifications.WithLoanTemplateSet(creditnotifications.LoanOverrides(cfg.Mobile.USSDDialString)),
	)
	if err != nil {
		log.Fatalf("Failed to create loan notifier: %v", err)
	}

	// ---- 10d. Loan product service ----
	loanProductSvc := loanproduct.NewService(repos.LoanProduct)

	// ---- 11. Link shortener ----
	//
	// Built before the adapters that take it: it is now a constructor
	// dependency rather than something applied afterwards, so it has to exist
	// first.
	var linkShortener urlshortener.Shortener
	if cfg.Shortener.Enabled() {
		linkShortener = urlshortener.NewDub(urlshortener.DubOptions{
			APIKey:             cfg.Shortener.APIKey,
			BaseURL:            cfg.Shortener.BaseURL,
			Domain:             cfg.Shortener.Domain,
			PreviewTitle:       cfg.Shortener.PreviewTitle,
			PreviewDescription: cfg.Shortener.PreviewDescription,
			ImagePreviewURL:    cfg.Shortener.ImagePreviewURL,
		})
		target := cfg.Shortener.BaseURL
		if target == "" {
			target = "https://api.dub.co"
		}
		slog.InfoContext(tomlCtx, "link shortener enabled", slog.String("target", target))
	}

	// Where borrower cash deposits are credited. The treasury, not a child
	// account: children carry no USDC trustline, so routing a deposit through
	// one would cost a sponsored reserve per borrower for an account that only
	// ever passes funds on.

	// ---- 11b. LoanServiceAdapter (USSD LoanService) ----
	ctx := context.Background()
	cashInRegistry := cashin.NewRegistry()
	loanAdapter, err := adapters.NewLoanServiceAdapter(ctx, adapters.LoanAdapterDeps{
		LoanSvc:      loanSvc,
		ProductSvc:   loanProductSvc,
		StellarSvc:   stellarSvc,
		OffRamps:     offRampRegistry,
		LoanNotifier: loanNotifier,
		TxnSvc:       txnSvc,
		FXConfig:     adapters.FXConfig{BufferPct: cfg.Payments.EntryFXBufferPct},
		Logger:       logger,
		CashIn:       cashInRegistry,

		RoundAnchorAmounts: cfg.Payments.RoundAnchorAmounts,
		FXOrchestrator:     fxOrch,
		PublicBaseURL:      cfg.Server.PublicBaseURL,
		Shortener:          linkShortener,
		AccountEnsurer:     userAdapter,
		RepaymentAnchor:    mgClient.Client,
		TreasuryAddress:    treasuryAddr,
		// The memo namespace follows the SEP-10 signer, which is the auth
		// wallet — the same address the poller derives from.
		AnchorAuthAddress: mgAuthAddr,
		RepaymentWindow:   cfg.Payments.MoneyGram.RepaymentWindow,
	})
	if err != nil {
		log.Fatalf("Failed to create loan service adapter: %v", err)
	}
	logger.InfoContext(ctx, "loan entry-rate buffer configured", "buffer_pct", loanAdapter.FXBufferPct())

	// MoneyGram is a cashin.Collector too — loanAdapter already owns the
	// anchor/treasury dependencies InitiateRepayment needs, so it registers
	// itself rather than a separate adapter duplicating them.
	if err := cashInRegistry.Register(loanAdapter); err != nil {
		log.Fatalf("Failed to register MoneyGram cash-in: %v", err)
	}
	if err := cashInRegistry.Alias(cashin.CollectionMethodCash, cashin.ProviderMoneyGram); err != nil {
		log.Fatalf("Failed to alias cash → moneygram: %v", err)
	}

	// ---- 12. DisbursementStatusAdapter (webhook callbacks) ----
	disbursementAdapter := adapters.NewDisbursementStatusAdapter(adapters.DisbursementAdapterDeps{
		Repo:          repos.Loan,
		LoanNotifier:  loanNotifier,
		TxnSvc:        txnSvc,
		StellarSvc:    stellarSvc,
		Logger:        logger,
		PublicBaseURL: cfg.Server.PublicBaseURL,
		Shortener:     linkShortener,

		TxResolver:            stellarrpc.NewVerifier(rpcClient),
		VaultRepayMaxAttempts: cfg.Stellar.VaultRepayMaxAttempts,
	})

	// ---- 12b. Rate service adapter ----
	// The USSD flow quotes from the same MG-primary/YC-fallback cascade the
	// loan adapter books against, so the rate shown at entry, the USDC the
	// treasury sends, and MoneyGram's cash-pickup floor are all measured with
	// one rate.
	rateSvc := adapters.NewRateServiceAdapter(fxOrch)

	// ---- 12c. Account notifier + PIN service ----
	accountNotifier, err := mvnotifications.NewSMSAccountNotifier(notifier,
		mvnotifications.WithAccountLanguageResolver(langResolver),
		mvnotifications.WithAccountTemplateSet(creditnotifications.AccountOverrides(cfg.Mobile.USSDDialString)),
	)
	if err != nil {
		log.Fatalf("Failed to create account notifier: %v", err)
	}
	pinRepo := pin.NewSecurityQuestionRepository(db)
	pinService := pin.NewService(coreRepos.User, pinRepo, accountNotifier, cfg.Auth.PINLockoutDuration)
	slog.InfoContext(ctx, "PIN service initialized")

	// ---- 13. USSD stack ----
	sessionManager := ussd.NewSessionManager(redisClient, cfg.Mobile.SessionTimeout)
	menuRegistry := ussd.NewMenuRegistry()

	// Register standard loan menus
	standardPreset := &ussd.StandardLoanMenuPreset{}
	standardPreset.Initialize(menuRegistry)

	repayPaybill := ""
	if cfg.Payments.Mpesa.CollectionShortcode > 0 {
		repayPaybill = strconv.FormatUint(uint64(cfg.Payments.Mpesa.CollectionShortcode), 10)
	}
	ussdHandler := ussd.NewUSSDHandler(ussd.HandlerDeps{
		SessionManager:  sessionManager,
		MenuRegistry:    menuRegistry,
		UserService:     userAdapter,
		LoanService:     loanAdapter,
		RateService:     rateSvc,
		PINService:      pinService,
		AccountNotifier: accountNotifier,
		LoanNotifier:    loanNotifier,
		RepayPaybill:    repayPaybill,
		MpesaPrompter:   true,
		// The Airtel rail appears in the repay menu only when the
		// integration is configured. The borrower picks their network; the
		// menu must not offer one that cannot be resolved.
		AirtelPrompter: cfg.Payments.Airtel.Enabled(),
	})
	ussdService := ussd.NewUSSDService(ussdHandler)

	// Register Africa's Talking USSD provider
	atUSSDProvider := atussd.NewAfricasTalkingUSSDAdapter(
		cfg.Mobile.AfricasTalking.Username,
		cfg.Mobile.AfricasTalking.APIKey,
	)
	ussdService.RegisterProvider("africastalking", atUSSDProvider)

	// ---- 13b. USSD controller ----
	ussdCtrl := controllers.NewUSSDController(ussdService)

	// ---- 14. Webhook service + controller ----
	webhookSvc := webhook.NewService(disbursementAdapter, nil, disbursementAdapter, ycAdapter)
	webhookCtrl := controllers.NewWebhookController(webhookSvc, cfg.Payments.YellowCard.PublicKey, cfg.Payments.YellowCard.SecretKey)

	// ---- 15. Pollers ----
	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	refundPoller := webhook.NewRefundPoller(
		ycAdapter,
		ycOffRamp,
		disbursementAdapter,
		disbursementAdapter,
		nil, // AlertService — ops alerts via logging for now
		disbursementAdapter,
		webhook.DefaultRefundPollerConfig(),
	)
	go refundPoller.Start(pollerCtx)

	// MoneyGram SEP-24 lifecycle poller.
	// Drives pending_user_transfer_start to SendUSDC, pending_user_transfer_complete
	// to backfill amount_out + cash-pickup reference, terminal to finalise.
	mgPollerAdapter, err := adapters.NewMoneyGramPollerAdapter(repos.Loan, loanSvc, txnSvc, logger)
	if err != nil {
		log.Fatalf("MoneyGram poller adapter construction failed: %v", err)
	}
	mgP, err := mgpoller.NewPoller(mgpoller.PollerDeps{
		Client:       mgClient,
		Fetcher:      mgPollerAdapter,
		Recorder:     mgPollerAdapter,
		Disbursement: disbursementAdapter,               // reused from the YC flow
		Treasury:     mgFundsTransfer,                   // funds-wallet sender for USDC to MG anchor
		Verifier:     stellarrpc.NewVerifier(rpcClient), // confirms MG's refunds landed on-ledger
		Alerts:       nil,                               // log-only for now
		Config:       mgPollerConfig(cfg),
		Logger:       logger,
	})
	if err != nil {
		log.Fatalf("MoneyGram poller construction failed: %v", err)
	}
	go mgP.Start(pollerCtx)
	slog.InfoContext(ctx, "MoneyGram poller started")

	// MoneyGram deposit driver — the borrower repayment cash-in rail.
	//
	// Second Driver on the same runner shape as the withdrawal poller above,
	// but on its own cadence: a deposit waits on the borrower rather than on
	// MoneyGram, so it is scheduled from repayment_next_poll_at rather than
	// polled hard.
	depositAdapter, err := adapters.NewMoneyGramDepositAdapter(adapters.DepositAdapterDeps{
		Repo:       repos.Loan,
		LoanSvc:    loanSvc,
		StellarSvc: stellarSvc,
		TxnSvc:     txnSvc,
		Logger:     logger,
	})
	if err != nil {
		log.Fatalf("MoneyGram deposit adapter construction failed: %v", err)
	}
	repaymentNotifier, err := adapters.NewRepaymentNotifierAdapter(repos.Loan, loanNotifier, cfg.Server.PublicBaseURL, linkShortener, offRampRegistry, logger)
	if err != nil {
		log.Fatalf("Repayment notifier construction failed: %v", err)
	}
	depositDriver, err := mgpoller.NewDepositDriver(mgpoller.DepositDriverDeps{
		Client:   mgClient,
		Fetcher:  depositAdapter,
		Recorder: depositAdapter,
		Vault:    depositAdapter,
		Notifier: repaymentNotifier,
		Alerts:   nil, // log-only for now, same as the withdrawal side
		Config:   mgPollerConfig(cfg),
		Logger:   logger,
	})
	if err != nil {
		log.Fatalf("MoneyGram deposit driver construction failed: %v", err)
	}
	go depositDriver.Start(pollerCtx)
	slog.InfoContext(ctx, "MoneyGram deposit driver started")

	// M-Pesa is a platform rail, wired unconditionally like MoneyGram and
	// YellowCard: a misconfigured Daraja credential is a boot failure, not a
	// silently-absent provider. The client constructor validates the
	// credentials; the adapters validate the rest of the config.
	//
	// The STK poller resolves Express observations into confirmed payments. A
	// callback only lands the receipt on the queue; the poller independently
	// verifies it before anything credits, per the confirm-before-credit
	// discipline.
	mpesaEnv := mpesa.EnvironmentSandbox
	if cfg.Server.ServerEnvironment == "production" {
		mpesaEnv = mpesa.EnvironmentProduction
	}
	mpesaClient, err := mpesa.New(mpesa.Config{
		Environment:         mpesaEnv,
		ConsumerKey:         cfg.Payments.Mpesa.ConsumerKey,
		ConsumerSecret:      cfg.Payments.Mpesa.ConsumerSecret,
		CollectionShortcode: cfg.Payments.Mpesa.CollectionShortcode,
		Passkey:             cfg.Payments.Mpesa.Passkey,
		InitiatorName:       cfg.Payments.Mpesa.InitiatorName,
		InitiatorPassword:   cfg.Payments.Mpesa.InitiatorPassword,
	})
	if err != nil {
		log.Fatalf("M-Pesa client construction failed: %v", err)
	}
	// Cash-in registry: paybill collections and STK prompts resolve to M-Pesa.
	// In-flight prompts are driven by the loan poller below.
	mpesaCollection, err := adapters.NewMpesaCollectionAdapter(adapters.MpesaCollectionAdapterDeps{
		Client:  mpesaClient,
		Repo:    repos.Loan,
		LoanSvc: loanSvc,
		Config:  cfg.Payments.Mpesa,

		UserSvc:          userSvc,
		ValidationRepo:   coreRepos.MpesaValidation,
		ValidationPolicy: cfg.Payments.Mpesa.NumberValidationPolicy,
		Logger:           logger,
	})
	if err != nil {
		log.Fatalf("M-Pesa collection adapter construction failed: %v", err)
	}
	if err := cashInRegistry.Register(mpesaCollection); err != nil {
		log.Fatalf("Failed to register M-Pesa cash-in: %v", err)
	}
	if err := cashInRegistry.Alias(cashin.CollectionMethodPayBill, cashin.ProviderMpesa); err != nil {
		log.Fatalf("Failed to alias pay_bill → mpesa: %v", err)
	}
	if err := cashInRegistry.Alias(cashin.CollectionMethodPrompt, cashin.ProviderMpesa); err != nil {
		log.Fatalf("Failed to alias prompt → mpesa: %v", err)
	}
	slog.InfoContext(ctx, "M-Pesa cash-in registered (cash-in providers: moneygram, mpesa)")

	mpesaLoanRunner, err := adapters.NewMpesaSTKLoanRunner(adapters.MpesaSTKLoanDriverDeps{
		Client:    mpesaClient,
		MpesaRepo: coreRepos.Mpesa,
		Repo:      repos.Loan,
		LoanSvc:   loanSvc,
		Notifier:  repaymentNotifier,
		Config:    cfg.Payments.Mpesa,
		Logger:    logger,
	})
	if err != nil {
		log.Fatalf("M-Pesa STK loan poller construction failed: %v", err)
	}
	go mpesaLoanRunner.Start(pollerCtx)
	slog.InfoContext(ctx, "M-Pesa STK loan poller started")

	// Paybill repayment sweep — walk-up-and-pay, so nothing "initiates" it
	// the way Prompt does for STK; this converts confirmed, loan-attributed
	// mpesa_transactions rows as they land. loanAdapter satisfies
	// repaymentQuoter (GetRepaymentQuote) for the lazy payoff lock.
	mpesaPaybillDriver, err := adapters.NewMpesaPaybillRepaymentDriver(adapters.MpesaPaybillRepaymentDriverDeps{
		MpesaRepo: coreRepos.Mpesa,
		Repo:      repos.Loan,
		Quoter:    loanAdapter,
		OffRamps:  offRampRegistry,
		Notifier:  repaymentNotifier,
		Interval:  cfg.Payments.Mpesa.PaybillSweepInterval,
		Logger:    logger,
	})
	if err != nil {
		log.Fatalf("M-Pesa paybill repayment driver construction failed: %v", err)
	}
	go mpesaPaybillDriver.Start(pollerCtx)
	slog.InfoContext(ctx, "M-Pesa paybill repayment sweep started")

	// Pull reconciliation sweep and Account Balance poll — both wall-clock
	// tickers, not Runner[T] drivers, since neither is a queue of due rows;
	// see pkg/services/mpesapoller/doc.go. Both run from this process because
	// the Daraja client they need is constructed here, not in core, even
	// though the package they're defined in is core's.
	pullSweeper := mpesapoller.NewPullSweeper(mpesapoller.PullSweeperDeps{
		Client:    mpesaClient,
		Repo:      coreRepos.Mpesa,
		Cursor:    coreRepos.MpesaPullCursor,
		Shortcode: cfg.Payments.Mpesa.CollectionShortcode,
		Interval:  cfg.Payments.Mpesa.PullSweepInterval,
		Logger:    logger,
	})
	go pullSweeper.Start(pollerCtx)
	slog.InfoContext(ctx, "M-Pesa pull sweeper started")

	balancePoller := mpesapoller.NewBalancePoller(mpesapoller.BalancePollerDeps{
		Client:                mpesaClient,
		Queries:               coreRepos.MpesaBalance,
		CollectionShortcode:   cfg.Payments.Mpesa.CollectionShortcode,
		DisbursementShortcode: cfg.Payments.Mpesa.DisbursementShortcode,
		ResultURL:             cfg.Payments.Mpesa.DarajaCallbackURL("balance/result"),
		QueueTimeOutURL:       cfg.Payments.Mpesa.DarajaCallbackURL("balance/timeout"),
		Interval:              cfg.Payments.Mpesa.BalancePollInterval,
		Logger:                logger,
	})
	go balancePoller.Start(pollerCtx)
	slog.InfoContext(ctx, "M-Pesa balance poller started")

	// Airtel Money is gated on its credentials rather than wired
	// unconditionally like M-Pesa: the rail is new, most deployments do not
	// have an Airtel developer account yet, and a boot failure for a rail
	// nobody is using would be a worse default than an absent one.
	//
	// Only the reconciliation sweep runs today. Confirming an individual
	// collection needs the loan that minted the transaction id, and that
	// adapter is not built — see the vault plan's open prompt-routing
	// decision.
	if cfg.Payments.Airtel.Enabled() {
		airtelClient, err := airtel.New(airtel.Config{
			Environment:     cfg.Payments.Airtel.Environment,
			ClientID:        cfg.Payments.Airtel.ClientID,
			ClientSecret:    cfg.Payments.Airtel.ClientSecret,
			Country:         cfg.Payments.Airtel.Country,
			Currency:        cfg.Payments.Airtel.Currency,
			SigningEnabled:  cfg.Payments.Airtel.SigningEnabled,
			CallbackHMACKey: cfg.Payments.Airtel.CallbackHMACKey,
		})
		if err != nil {
			log.Fatalf("Airtel client construction failed: %v", err)
		}

		// The collection adapter is registered but deliberately not aliased
		// to any CollectionMethod: the method aliases stay pointed at
		// M-Pesa, and the Airtel rail is reached by the borrower naming it
		// in the repay menu, which resolves the provider by id.
		airtelCollection, err := adapters.NewAirtelCollectionAdapter(adapters.AirtelCollectionAdapterDeps{
			Client:  airtelClient,
			Repo:    repos.Loan,
			LoanSvc: loanSvc,
			Config:  cfg.Payments.Airtel,
			Logger:  logger,
		})
		if err != nil {
			log.Fatalf("Airtel collection adapter construction failed: %v", err)
		}
		if err := cashInRegistry.Register(airtelCollection); err != nil {
			log.Fatalf("Failed to register Airtel cash-in: %v", err)
		}

		airtelLoanRunner, err := adapters.NewAirtelPromptLoanRunner(adapters.AirtelPromptLoanDriverDeps{
			Client:     airtelClient,
			AirtelRepo: coreRepos.Airtel,
			Repo:       repos.Loan,
			LoanSvc:    loanSvc,
			Notifier:   repaymentNotifier,
			Config:     cfg.Payments.Airtel,
			Logger:     logger,
		})
		if err != nil {
			log.Fatalf("Airtel prompt loan poller construction failed: %v", err)
		}
		go airtelLoanRunner.Start(pollerCtx)
		slog.InfoContext(ctx, "Airtel prompt loan poller started")

		airtelSweeper := airtelpoller.NewSummarySweeper(airtelpoller.SummarySweeperDeps{
			Client:   airtelClient,
			Repo:     coreRepos.Airtel,
			Cursor:   coreRepos.AirtelSummaryCursor,
			Interval: cfg.Payments.Airtel.SummarySweepInterval,
			Logger:   logger,
		})
		go airtelSweeper.Start(pollerCtx)
		slog.InfoContext(ctx, "Airtel summary sweeper started")
	}

	// Compliance watcher — the detect-and-quarantine canary alongside the
	// vault's on-chain allowlist. See pkg/services/vaultwatch/doc.go.
	vaultWatcher := vaultwatch.NewWatcher(vaultwatch.WatcherDeps{
		Client:     rpcClient,
		Allowlist:  stellarSvc,
		Cursor:     coreRepos.VaultWatchCursor,
		ContractID: cfg.Stellar.ContractID,
		Interval:   cfg.Stellar.VaultWatchInterval,
		Logger:     logger,
	})
	go vaultWatcher.Start(pollerCtx)
	slog.InfoContext(ctx, "vault compliance watcher started")

	// On-chain writer + rescreening sweep — Phases 5/6. Deliberately built
	// here, not in cmd/admin: the compliance role signing key must stay out
	// of the web process (source design doc §14). The admin writes intent
	// to counterparty_addresses; these two tickers are what actually acts
	// on it.
	ellipticClient := elliptic.NewClient(elliptic.Config{
		APIKey:     cfg.Compliance.EllipticAPIKey,
		APISecret:  cfg.Compliance.EllipticAPISecret,
		BaseURL:    cfg.Compliance.EllipticBaseURL,
		Thresholds: elliptic.Thresholds{}, // runtime-configurable per the source design doc §17 Q8 — not wired to an admin control yet
	})
	complianceService := compliancesvc.NewService(compliancesvc.Deps{
		Repo:              coreRepos.Counterparty,
		Screener:          ellipticClient,
		ScreeningValidity: cfg.Compliance.ScreeningValidity,
		Logger:            logger,
	})

	complianceSigner := stellarSvc.WithComplianceRole(cfg.Compliance.ComplianceRoleSecretKey)
	onchainWriter := compliancesvc.NewOnchainWriter(compliancesvc.OnchainWriterDeps{
		Repo:     coreRepos.Counterparty,
		Signer:   complianceSigner,
		Interval: cfg.Compliance.OnchainWriterInterval,
		Logger:   logger,
	})
	go onchainWriter.Start(pollerCtx)
	slog.InfoContext(ctx, "compliance on-chain writer started")

	rescreenSweep := compliancesvc.NewRescreenSweep(compliancesvc.RescreenSweepDeps{
		Repo:     coreRepos.Counterparty,
		Service:  complianceService,
		Interval: cfg.Compliance.RescreenSweepInterval,
		Logger:   logger,
	})
	go rescreenSweep.Start(pollerCtx)
	slog.InfoContext(ctx, "compliance rescreen sweep started")

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get the database handle for the vault repay reconciler: %v", err)
	}
	vaultRepayRunner, err := adapters.NewVaultRepayReconcileRunner(adapters.VaultRepayReconcilerDeps{
		Repo:             repos.Loan,
		Reconciler:       disbursementAdapter,
		DB:               sqlDB,
		Logger:           logger,
		Interval:         cfg.Stellar.VaultRepayReconcileInterval,
		SettlementWindow: cfg.Stellar.VaultRepaySettlementWindow,
		RetryBackoff:     cfg.Stellar.VaultRepayRetryBackoff,
		MaxAttempts:      cfg.Stellar.VaultRepayMaxAttempts,
	})
	if err != nil {
		log.Fatalf("Failed to create the vault repay reconciler: %v", err)
	}
	go vaultRepayRunner.Start(pollerCtx)
	slog.InfoContext(ctx, "vault repay reconciler started")

	accountHealRunner, err := accountheal.NewRunner(accountheal.Deps{
		Ensurer:      userAdapter,
		Repo:         coreRepos.Account,
		DB:           sqlDB,
		Logger:       logger,
		Interval:     cfg.Stellar.AccountHealInterval,
		RetryAfter:   cfg.Stellar.AccountHealRetryAfter,
		RetryBackoff: cfg.Stellar.AccountHealRetryBackoff,
		PendingAfter: cfg.Stellar.AccountHealPendingAfter,
		MaxAttempts:  cfg.Stellar.AccountHealMaxAttempts,
	})
	if err != nil {
		log.Fatalf("Failed to create the account chain reconciler: %v", err)
	}
	go accountHealRunner.Start(pollerCtx)
	slog.InfoContext(ctx, "account chain reconciler started")

	// ---- 16. Fiber app + middleware + routes ----
	// The proxy header is read only from a trusted hop: without the
	// trusted-proxy check, any client reaching the port could set
	// X-Forwarded-For and choose the address the Daraja allowlist sees.
	app := fiber.New(fiber.Config{
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          cfg.Server.TrustedProxyCIDRs,
	})

	healthCheck := health.NewCheckerWithoutStellar("credit", "credit")
	middleware.FiberMiddleware(app, healthCheck, logger)

	// Swagger
	app.Get("/swagger/*", swagger.New(swagger.Config{
		DeepLinking:  false,
		DocExpansion: "list",
		OAuth: &swagger.OAuthConfig{
			AppName:  "OAuth Provider",
			ClientId: "21bb4edc-05a7-4afc-86f1-2e151e4ba6e2",
		},
		OAuth2RedirectUrl: "http://localhost:8081/swagger/oauth2-redirect.html",
	}))

	// Redoc at root
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./cmd/credit/docs/redoc-static.html")
	})

	// Core API routes (from microvault)
	api := app.Group("/api/v1")
	api.Get("/auth/challenge", middleware.FormatResponse(), authController.GetChallenge)
	api.Post("/auth/verify", middleware.FormatResponse(), authController.VerifyChallenge)

	// USSD callback — supports multiple providers via URL param
	api.Post("/mobile/ussd/:provider", ussdCtrl.HandleCallback)

	// SMS delivery report callback — supports multiple providers via URL param
	smsCallbackHandler := sms.NewDeliveryReportHandler()
	smsCallbackCtrl := controllers.NewSMSCallbackController(smsCallbackHandler)
	api.Post("/mobile/sms/:provider/delivery", smsCallbackCtrl.HandleDeliveryReport)

	// Webhook routes
	api.Post("/webhooks/yellowcard", webhookCtrl.HandleYellowCardWebhook)

	// Daraja callbacks — the STK result URL the prompt adapter builds is
	// CallbackBaseURL + /api/v1/callbacks/daraja/{slug}/stk/result, and that
	// base (MPESA_CALLBACK_BASE_URL) is the bare host for this server. The
	// adapter adds the /api/v1 prefix, so these mount on the /api/v1 group.
	// Unauthenticated by design: Daraja signs nothing, so the unguessable slug
	// and the source-IP allowlist are the controls.
	if cfg.Payments.Mpesa.CallbackSlug != "" {
		resolveLoan := func(ctx context.Context, reference string) (string, error) {
			return coreRepos.Mpesa.GetLoanIDByReference(ctx, reference)
		}
		darajaCtrl := controllers.NewDarajaCallbackController(
			coreRepos.Mpesa, cfg.Payments.Mpesa, cfg.Server.ServerEnvironment, resolveLoan)
		darajaCtrl.Register(api)
	}

	// Cash-pickup SMS short-link → MoneyGram interactive URL redirect.
	//
	// Rate limited: the code is an unauthenticated 40-bit bearer token resolving
	// to a live KYC session, and finding *any* live code is far cheaper than
	// finding a given one. A borrower taps their link a handful of times; a
	// scanner does not.
	//
	// Backed by Redis rather than the default in-memory store so the ceiling is
	// 20/min in total, not 20/min per replica. Sliding rather than fixed window:
	// a fixed one lets 40 through across a window boundary.
	redirectHandler := handlers.NewInteractiveRedirectHandler(loanSvc, logger)
	app.Get("/r/:code", limiter.New(limiter.Config{
		Max:               20,
		Expiration:        time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		Storage:           ratelimit.NewRedisStore(redisClient, "microvault:ratelimit:redirect"),
		LimitReached: func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusTooManyRequests)
		},
	}), redirectHandler.Handle)

	// ---- 17. Start server ----
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.InfoContext(ctx, "starting credit server", slog.String("credit_addr", cfg.Server.CreditAddr()))
		if err := app.Listen(cfg.Server.CreditAddr()); err != nil {
			slog.ErrorContext(ctx, "Server Listen error", slog.Any("error", err))
		}
	}()

	// ---- 18. Graceful shutdown ----
	//
	// Teardown order is the container's, not this function's: resources shut
	// down in reverse of the order newLifecycle provided them, so the pollers
	// stop before the database and cache they read through. See lifecycle.go.
	sig := <-sigChan
	slog.InfoContext(ctx, "received signal, shutting down", slog.String("signal", sig.String()))

	lifecycle := newLifecycle(pollerCancel, app, shutdownTracing)
	if errs := lifecycle.Shutdown(); errs != nil {
		slog.ErrorContext(ctx, "Shutdown completed with errors", slog.Any("error", errs))
	}

	slog.InfoContext(ctx, "Application shutdown complete")
}

// mgPollerConfig overlays the MoneyGram poller settings from the environment
// onto the package defaults. Unset values stay zero and NewPoller keeps its
// own default, so config never duplicates the defaults.
func mgPollerConfig(cfg *config.Config) mgpoller.PollerConfig {
	c := mgpoller.DefaultConfig()
	mg := cfg.Payments.MoneyGram
	if mg.PollInterval > 0 {
		c.PollInterval = mg.PollInterval
	}
	if mg.PollMaxBatch > 0 {
		c.MaxBatch = mg.PollMaxBatch
	}
	if mg.RefundSettleMaxAttempts > 0 {
		c.RefundSettleMaxAttempts = mg.RefundSettleMaxAttempts
	}
	// MoneyGram refunds return to the SEP-24 funds wallet, which is a different
	// account from the SEP-10 auth wallet outside development. Left unset, the
	// poller would watch the auth account and no refund would ever settle.
	if addr, err := mg.FundsAddress(); err == nil {
		c.RefundDestination = addr
	} else {
		slog.Error("MoneyGram funds address unresolved, refund destination falls back to the SEP-10 account", slog.Any("error", err))
	}
	// Borrower repayment cash-in. Left at DefaultConfig's values when unset,
	// rather than config restating the defaults.
	if mg.RepaymentPollInterval > 0 {
		c.DepositPollInterval = mg.RepaymentPollInterval
	}
	if mg.RepaymentReminderBefore > 0 {
		c.DepositReminderBefore = mg.RepaymentReminderBefore
	}
	if mg.RepaymentVaultMaxAttempt > 0 {
		c.DepositVaultMaxAttempts = mg.RepaymentVaultMaxAttempt
	}
	// The per-row schedule. DepositPollInterval only sets how often the runner
	// asks; these decide what it gets back, so both have to shrink together for
	// a development deposit to move quickly.
	if mg.RepaymentActiveBackoff > 0 {
		c.DepositActiveBackoff = mg.RepaymentActiveBackoff
	}
	if mg.RepaymentIdleBackoff > 0 {
		c.DepositIdleBackoff = mg.RepaymentIdleBackoff
	}
	if mg.RepaymentVaultRetryBackoff > 0 {
		c.DepositVaultRetryBackoff = mg.RepaymentVaultRetryBackoff
	}
	c.RefundAssetIssuer = cfg.Stellar.USDCIssuer
	return c
}
