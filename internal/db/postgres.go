package db

import (
	"context"
	"fmt"

	"binancebot/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store veritabanı işlemleri için tek nokta. Global state yok; test ve kullanım kolay.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore bağlantı kurar, tabloları oluşturur ve Store döner.
func NewStore(ctx context.Context, cfg *config.Config) (*Store, error) {
	connStr := cfg.Postgres.URL
	if connStr == "" {
		connStr = fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=disable",
			cfg.Postgres.User, cfg.Postgres.Password,
			cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DB,
		)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres bağlantı: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// Close bağlantı havuzunu kapatır.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS trades (
			id                   BIGSERIAL PRIMARY KEY,
			symbol               VARCHAR(32) NOT NULL,
			side                 VARCHAR(8) NOT NULL,
			position_side        VARCHAR(8),
			entry_price          DECIMAL(20,8) NOT NULL,
			quantity             DECIMAL(20,8) NOT NULL,
			stop_loss            DECIMAL(20,8),
			take_profit          DECIMAL(20,8),
			binance_order_id     VARCHAR(64),
			balance_before_usdt  DECIMAL(20,8),
			balance_after_usdt   DECIMAL(20,8),
			opened_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			closed_at            TIMESTAMPTZ,
			exit_price          DECIMAL(20,8),
			realized_pnl         DECIMAL(20,8),
			close_reason         VARCHAR(32),
			created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol);
		CREATE INDEX IF NOT EXISTS idx_trades_opened_at ON trades(opened_at);
		CREATE INDEX IF NOT EXISTS idx_trades_closed_at ON trades(closed_at);
	`)
	if err != nil {
		return err
	}
	// Yeni sütunlar (mevcut DB'ler için)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS balance_before_usdt DECIMAL(20,8)`)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS balance_after_usdt DECIMAL(20,8)`)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS instance_id VARCHAR(128)`)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS leverage INT DEFAULT 1`)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS margin_usdt DECIMAL(20,8)`)
	_, _ = s.pool.Exec(ctx, `ALTER TABLE trades ADD COLUMN IF NOT EXISTS notional_usdt DECIMAL(20,8)`)

	// Pozisyon açma hataları (log için ayrı tablo)
	_, err = s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS position_open_errors (
			id            BIGSERIAL PRIMARY KEY,
			symbol        VARCHAR(32) NOT NULL,
			side          VARCHAR(16) NOT NULL,
			error_message TEXT NOT NULL,
			instance_id   VARCHAR(128),
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_position_open_errors_symbol ON position_open_errors(symbol);
		CREATE INDEX IF NOT EXISTS idx_position_open_errors_created_at ON position_open_errors(created_at);
	`)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS runtime_config (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
	`)
	return err
}
