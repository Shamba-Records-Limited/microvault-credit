package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/do/v2"

	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
)

// This file holds the process lifecycle: what must be torn down, and in what
// order.
//
// The container owns shutdown rather than construction. Most of this service's
// wiring is a straight line — config, then database, then services that depend
// on them — and expressing that as lazy providers would restate the order
// without enforcing anything new. Teardown is the part that genuinely benefits:
// it has to run in reverse dependency order, every step has to run even when an
// earlier one fails, and getting it wrong drops in-flight work silently.
//
// do.Injector gives that ordering for free through Shutdowner, and makes each
// resource state its own teardown next to nothing else.

// shutdownFunc adapts a plain teardown closure to do's Shutdowner interface, so
// resources whose own types this module does not own — Fiber's app, the
// process-wide database and cache registries — can still be registered.
type shutdownFunc struct {
	name string
	stop func() error
}

// Shutdown implements do.ShutdownerWithError.
func (s *shutdownFunc) Shutdown() error {
	if err := s.stop(); err != nil {
		// Logged rather than returned upward: one resource failing to close
		// must not stop the rest from being closed.
		log.Printf("%s shutdown error: %v", s.name, err)
		return err
	}
	log.Printf("%s shut down.", s.name)
	return nil
}

// pollerGroup is the background poller set. It shuts down first, before the
// database and cache it reads through.
type pollerGroup struct {
	cancel context.CancelFunc
}

// Shutdown implements do.ShutdownerWithError.
func (p *pollerGroup) Shutdown() error {
	p.cancel()
	log.Println("Pollers stopped.")
	return nil
}

// newLifecycle registers every resource that needs ordered teardown.
//
// Registration order is teardown order reversed: do shuts services down in the
// reverse of the order they were provided, which is what puts the pollers ahead
// of the connections they use.
func newLifecycle(pollerCancel context.CancelFunc, app *fiber.App) do.Injector {
	injector := do.New()

	// Provided first, so torn down last: everything above depends on these.
	do.ProvideNamedValue(injector, "cache", &shutdownFunc{name: "Cache connections", stop: cache.CloseAll})
	do.ProvideNamedValue(injector, "database", &shutdownFunc{name: "Database connections", stop: database.CloseAll})

	// The HTTP server stops accepting before the pollers stop working, so a
	// request already in flight still has its dependencies.
	do.ProvideNamedValue(injector, "http", &shutdownFunc{name: "Fiber server", stop: app.Shutdown})

	// Provided last, so torn down first.
	do.ProvideNamedValue(injector, "pollers", &pollerGroup{cancel: pollerCancel})

	return injector
}
