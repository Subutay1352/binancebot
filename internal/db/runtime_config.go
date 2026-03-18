package db

import (
	"context"
)

// RuntimeConfigKeys UI'dan değiştirilebilir ayar anahtarları (env ile aynı isim).
var RuntimeConfigKeys = []string{
	"STOP_LOSS_PERCENT", "TAKE_PROFIT_PERCENT",
	"COIN_COOLDOWN_MIN", "MAX_TRADE_DURATION_MIN",
	"STRATEGY_INTERVAL", "STRATEGY_RSI_PERIOD", "STRATEGY_RSI_LOW", "STRATEGY_RSI_HIGH", "STRATEGY_RSI_SLOPE_ENABLE",
	"STRATEGY_MIN_VOLUME_USD", "STRATEGY_VOLUME_RATIO", "STRATEGY_VOLUME_AVG_PERIOD", "STRATEGY_VOLUME_MIN_RATIO_AVG",
	"MAX_DIRECTION_WEIGHT",
}

// GetRuntimeConfig DB'deki runtime ayarlarını döner (key -> value).
func (s *Store) GetRuntimeConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM runtime_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SaveRuntimeConfig verilen anahtarları runtime_config tablosuna yazar (sadece RuntimeConfigKeys içindekiler).
func (s *Store) SaveRuntimeConfig(ctx context.Context, values map[string]string) error {
	allowed := make(map[string]bool)
	for _, k := range RuntimeConfigKeys {
		allowed[k] = true
	}
	for key, val := range values {
		if !allowed[key] {
			continue
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO runtime_config (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		`, key, val)
		if err != nil {
			return err
		}
	}
	return nil
}
