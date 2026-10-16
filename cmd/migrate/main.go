// Command migrate applies or rolls back SQL migrations from the migrations/
// directory. Bookkeeping lives in schema_migrations; a PostgreSQL advisory
// lock serializes concurrent runners (e.g. several API replicas booting).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sr-alasgarov/wallet-gateway/internal/db"
	"github.com/sr-alasgarov/wallet-gateway/internal/observability"
)

const advisoryLockID = 8274619283746152

type migration struct {
	version int
	name    string
	upSQL   string
	downSQL string
}

func main() {
	var (
		direction   = flag.String("direction", "up", "up or down")
		steps       = flag.Int("steps", 0, "number of migrations to apply (0 = all for up, 1 for down)")
		dir         = flag.String("dir", "migrations", "migrations directory")
		databaseURL = flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL")
	)
	flag.Parse()

	logger := observability.NewLogger(envOr("LOG_LEVEL", "info"), envOr("LOG_FORMAT", "text"), "migrate")

	if *databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, db.Config{URL: *databaseURL, MaxConns: 2})
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	migrations, err := loadMigrations(*dir)
	if err != nil {
		logger.Error("load migrations", "error", err)
		os.Exit(1)
	}
	if len(migrations) == 0 {
		logger.Error("no migrations found", "dir", *dir)
		os.Exit(1)
	}
	if err := run(ctx, pool, migrations, *direction, *steps, logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, pool *pgxpool.Pool, migrations []migration, direction string, steps int, logger *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(advisoryLockID)); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, int64(advisoryLockID))
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    BIGINT PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn.Conn())
	if err != nil {
		return err
	}

	switch direction {
	case "up":
		count := 0
		for _, m := range migrations {
			if applied[m.version] {
				continue
			}
			if steps > 0 && count >= steps {
				break
			}
			if err := applyOne(ctx, conn.Conn(), m, true); err != nil {
				return fmt.Errorf("apply %04d_%s: %w", m.version, m.name, err)
			}
			logger.Info("migration applied", "version", m.version, "name", m.name)
			count++
		}
		if count == 0 {
			logger.Info("database is up to date")
		}
		return nil

	case "down":
		if steps <= 0 {
			steps = 1
		}
		sorted := make([]migration, len(migrations))
		copy(sorted, migrations)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].version > sorted[j].version })
		count := 0
		for _, m := range sorted {
			if !applied[m.version] || count >= steps {
				continue
			}
			if m.downSQL == "" {
				return fmt.Errorf("migration %04d_%s has no down file", m.version, m.name)
			}
			if err := applyOne(ctx, conn.Conn(), m, false); err != nil {
				return fmt.Errorf("rollback %04d_%s: %w", m.version, m.name, err)
			}
			logger.Info("migration rolled back", "version", m.version, "name", m.name)
			count++
		}
		if count == 0 {
			logger.Info("nothing to roll back")
		}
		return nil

	default:
		return fmt.Errorf("unknown direction %q (use up or down)", direction)
	}
}

func appliedVersions(ctx context.Context, conn *pgx.Conn) (map[int]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applyOne runs one migration and its bookkeeping in a single transaction.
func applyOne(ctx context.Context, conn *pgx.Conn, m migration, up bool) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	sql := m.upSQL
	if !up {
		sql = m.downSQL
	}
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if up {
		_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.version)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// loadMigrations reads NNNN_name.up.sql / NNNN_name.down.sql pairs.
func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	byVersion := map[int]*migration{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()
		var suffix string
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			suffix = ".up.sql"
		case strings.HasSuffix(name, ".down.sql"):
			suffix = ".down.sql"
		default:
			continue
		}
		base := strings.TrimSuffix(name, suffix)
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed migration filename %q (expected NNNN_name%s)", name, suffix)
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("malformed migration version in %q: %w", name, err)
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		m := byVersion[version]
		if m == nil {
			m = &migration{version: version, name: parts[1]}
			byVersion[version] = m
		}
		if suffix == ".up.sql" {
			m.upSQL = string(content)
		} else {
			m.downSQL = string(content)
		}
	}
	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.upSQL == "" {
			return nil, fmt.Errorf("migration %04d_%s is missing its .up.sql file", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
