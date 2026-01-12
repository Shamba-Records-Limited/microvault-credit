package database

import (
	"fmt"
	"log"

	"github.com/Shamba-Records-Limited/Microvault/pkg/config"
	microvaultdb "github.com/Shamba-Records-Limited/Microvault/platform/database"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations executes all pending database migrations (core + credit)
func RunMigrations(cfg *config.PostgresConfig) error {
	// Run core migrations first
	log.Println("Running core migrations...")
	if err := microvaultdb.RunMigrations(cfg); err != nil {
		return fmt.Errorf("failed to run core migrations: %w", err)
	}

	// Run credit migrations
	log.Println("Running credit migrations...")
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
			log.Printf("Error closing migrate source: %v", sourceErr)
		}
		if dbErr != nil {
			log.Printf("Error closing migrate database: %v", dbErr)
		}
	}()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if err == migrate.ErrNoChange {
		log.Println("Credit migrations: no changes to apply")
	} else {
		log.Println("Credit migrations: applied successfully")
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
			log.Printf("Error closing migrate source: %v", sourceErr)
		}
		if dbErr != nil {
			log.Printf("Error closing migrate database: %v", dbErr)
		}
	}()

	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	if err == migrate.ErrNoChange {
		log.Println("Credit rollback: no migrations to rollback")
	} else {
		log.Println("Credit rollback: completed successfully")
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
			log.Printf("Error closing migrate source: %v", sourceErr)
		}
		if dbErr != nil {
			log.Printf("Error closing migrate database: %v", dbErr)
		}
	}()

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
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
			log.Printf("Error closing migrate source: %v", sourceErr)
		}
		if dbErr != nil {
			log.Printf("Error closing migrate database: %v", dbErr)
		}
	}()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version: %w", err)
	}

	log.Printf("Credit migration: forced to version %d", version)
	return nil
}
