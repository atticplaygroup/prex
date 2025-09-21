package store_test

import (
	"context"
	"database/sql"
	"log"
	"testing"

	"github.com/atticplaygroup/prex/internal/config"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	migrate "github.com/rubenv/sql-migrate"
)

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)

	conf := config.LoadConfig("../../.env")

	var err error
	StoreTestDb, err = sql.Open("postgres", conf.TestDbUrl)
	if err != nil {
		log.Fatalf("Failed to open db: %v\n", err)
	}
	defer StoreTestDb.Close()

	// Check if the database is reachable
	if err := StoreTestDb.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v\n", err)
	}

	// Define the migration source
	Migrations = &migrate.FileMigrationSource{
		Dir: conf.TestMigrateSourceUrl,
	}

	ctx := context.Background()

	conn, err := pgxpool.New(ctx, conf.TestDbUrl)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v\n", err)
	}
	defer conn.Close()
	StoreInstance = store.NewStore(conn)

	RunSpecs(t, "Store Suite")
}

var (
	StoreTestDb   *sql.DB
	Migrations    *migrate.FileMigrationSource
	StoreInstance *store.Store
)

func RefreshDb(testDb *sql.DB, migrations *migrate.FileMigrationSource) {
	if _, err := migrate.Exec(testDb, "postgres", migrations, migrate.Down); err != nil {
		log.Fatalf("Failed to migrate down: %v\n", err)
	}
	if _, err := migrate.Exec(testDb, "postgres", migrations, migrate.Up); err != nil {
		log.Fatalf("Failed to migrate up: %v\n", err)
	}
}
