package runtime

import (
	"context"
	"database/sql"
	"time"
)

// DBConfig defines connection parameters for popular databases (PostgreSQL, MySQL, SQLite, etc.).
type DBConfig struct {
	DriverName      string
	DataSourceName  string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// ConnectDatabase initializes a configured, thread-safe database connection pool.
func ConnectDatabase(cfg DBConfig) (*sql.DB, error) {
	db, err := sql.Open(cfg.DriverName, cfg.DataSourceName)
	if err != nil {
		return nil, err
	}

	// Apply connection pool sizing limits
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(100)
	}

	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(10)
	}

	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(1 * time.Hour)
	}

	// Verify target host is reachable
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
