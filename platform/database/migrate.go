package database

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	microvaultdb "github.com/Shamba-Records-Limited/microvault/platform/database"
)

// RunMigrations executes all pending database migrations (core + credit)
func RunMigrations(cfg *config.PostgresConfig) error {
	// Run core migrations first
	slog.Info("Running core migrations")
	if err := microvaultdb.RunMigrations(cfg); err != nil {
		return fmt.Errorf("failed to run core migrations: %w", err)
	}

	// Run credit migrations
	slog.Info("Running credit migrations")
	if err := runCreditMigrations(cfg); err != nil {
		return fmt.Errorf("failed to run credit migrations: %w", err)
	}

	return nil
}

// runCreditMigrations executes credit-specific migrations
func runCreditMigrations(cfg *config.PostgresConfig) error {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&x-migrations-table=credit_schema_migrations",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)

	source, err := iofs.New(CreditMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("Error closing migrate source", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Error("Error closing migrate database", slog.Any("error", dbErr))
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("Credit migrations: no changes to apply")
	} else {
		slog.Info("Credit migrations: applied successfully")
	}

	return nil
}

// RollbackMigration rolls back the last credit migration
func RollbackMigration(cfg *config.PostgresConfig) error {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&x-migrations-table=credit_schema_migrations",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)

	source, err := iofs.New(CreditMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("Error closing migrate source", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Error("Error closing migrate database", slog.Any("error", dbErr))
		}
	}()

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("Credit rollback: no migrations to rollback")
	} else {
		slog.Info("Credit rollback: completed successfully")
	}

	return nil
}

// MigrationVersion returns the current credit migration version
func MigrationVersion(cfg *config.PostgresConfig) (uint, bool, error) {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&x-migrations-table=credit_schema_migrations",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)

	source, err := iofs.New(CreditMigrations, "migrations")
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("Error closing migrate source", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Error("Error closing migrate database", slog.Any("error", dbErr))
		}
	}()

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

// ForceMigrationVersion sets the credit migration version without running migrations
func ForceMigrationVersion(cfg *config.PostgresConfig, version int) error {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&x-migrations-table=credit_schema_migrations",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)

	source, err := iofs.New(CreditMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("Error closing migrate source", slog.Any("error", sourceErr))
		}
		if dbErr != nil {
			slog.Error("Error closing migrate database", slog.Any("error", dbErr))
		}
	}()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version: %w", err)
	}

	slog.Info("Credit migration: forced to version", slog.Int("version", version))
	return nil
}
