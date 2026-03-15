package db

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// strPtr boş değilse *string döner, boşsa nil (COALESCE sonrası kullanım için).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Trade tek bir işlem kaydı. JSON alanları UI/API ile uyumlu snake_case.
type Trade struct {
	ID                 int64      `json:"id"`
	Symbol             string     `json:"symbol"`
	Side               string     `json:"side"`
	PositionSide       *string    `json:"position_side"`
	EntryPrice         float64    `json:"entry_price"`
	Quantity           float64    `json:"quantity"`
	StopLoss           *float64   `json:"stop_loss"`
	TakeProfit         *float64   `json:"take_profit"`
	BinanceOrderID     *string    `json:"binance_order_id"`
	BalanceBeforeUsdt *float64   `json:"balance_before_usdt"`
	BalanceAfterUsdt  *float64   `json:"balance_after_usdt"`
	OpenedAt           time.Time  `json:"opened_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	ExitPrice          *float64   `json:"exit_price"`
	RealizedPnl        *float64   `json:"realized_pnl"`
	CloseReason        *string    `json:"close_reason"`
	InstanceID         *string    `json:"instance_id"` // Hangi bot instance yazdı (hostname veya BOT_INSTANCE_ID)
	Leverage           int        `json:"leverage"`    // Kaldıraç (1–125); 0 = eski kayıt
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// InsertTrade yeni açık pozisyon kaydı ekler, id döner.
func (s *Store) InsertTrade(ctx context.Context, t *Trade) (int64, error) {
	log.Printf("[db] INSERT trade | symbol=%s side=%s entry=%.4f qty=%.6f balance_before=%.2f instance=%s", t.Symbol, t.Side, t.EntryPrice, t.Quantity, ptrFloat(t.BalanceBeforeUsdt), strVal(t.InstanceID))
	lev := t.Leverage
	if lev < 1 {
		lev = 1
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO trades (symbol, side, position_side, entry_price, quantity, stop_loss, take_profit, binance_order_id, balance_before_usdt, opened_at, instance_id, leverage)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`,
		t.Symbol, t.Side, strVal(t.PositionSide), t.EntryPrice, t.Quantity, t.StopLoss, t.TakeProfit, strVal(t.BinanceOrderID), t.BalanceBeforeUsdt, t.OpenedAt, t.InstanceID, lev,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err == nil {
		log.Printf("[db] INSERT OK | trade_id=%d", t.ID)
	}
	return t.ID, err
}

// CloseTrade pozisyonu kapatır (çıkış fiyatı, PnL, sebep, işlem sonrası bakiye yazılır).
func (s *Store) CloseTrade(ctx context.Context, id int64, exitPrice, realizedPnl float64, balanceAfterUsdt *float64, closeReason string) error {
	log.Printf("[db] CLOSE trade | trade_id=%d exit=%.8f pnl=%.4f balance_after=%.2f reason=%s", id, exitPrice, realizedPnl, ptrFloat(balanceAfterUsdt), closeReason)
	_, err := s.pool.Exec(ctx, `
		UPDATE trades SET closed_at = $1, exit_price = $2, realized_pnl = $3, balance_after_usdt = $4, close_reason = $5, updated_at = $1 WHERE id = $6
	`, time.Now(), exitPrice, realizedPnl, balanceAfterUsdt, closeReason, id)
	if err == nil {
		log.Printf("[db] CLOSE OK | trade_id=%d", id)
	}
	return err
}

func ptrFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// StrVal pointer string'i güvenle döner (API/diğer paketler için).
func StrVal(s *string) string { return strVal(s) }

// OpenTrades tüm açık pozisyonları döner (çoklu sembol için).
func (s *Store) OpenTrades(ctx context.Context) ([]Trade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, symbol, side, COALESCE(position_side, ''), entry_price, quantity, stop_loss, take_profit, COALESCE(binance_order_id, ''), balance_before_usdt, balance_after_usdt, opened_at,
		       closed_at, exit_price, realized_pnl, COALESCE(close_reason, ''), COALESCE(leverage, 1), created_at, updated_at
		FROM trades WHERE closed_at IS NULL ORDER BY opened_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Trade
	for rows.Next() {
		var t Trade
		var posSide, ordID, closeReason string
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &posSide, &t.EntryPrice, &t.Quantity, &t.StopLoss, &t.TakeProfit,
			&ordID, &t.BalanceBeforeUsdt, &t.BalanceAfterUsdt, &t.OpenedAt, &t.ClosedAt, &t.ExitPrice, &t.RealizedPnl, &closeReason, &t.Leverage, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.PositionSide = strPtr(posSide)
		t.BinanceOrderID = strPtr(ordID)
		t.CloseReason = strPtr(closeReason)
		list = append(list, t)
	}
	return list, rows.Err()
}

// OpenTradeBySymbol sembole göre açık pozisyon döner. Yoksa nil, nil.
func (s *Store) OpenTradeBySymbol(ctx context.Context, symbol string) (*Trade, error) {
	var t Trade
	var posSide, ordID, closeReason string
	err := s.pool.QueryRow(ctx, `
		SELECT id, symbol, side, COALESCE(position_side, ''), entry_price, quantity, stop_loss, take_profit, COALESCE(binance_order_id, ''), balance_before_usdt, balance_after_usdt, opened_at,
		       closed_at, exit_price, realized_pnl, COALESCE(close_reason, ''), COALESCE(leverage, 1), created_at, updated_at
		FROM trades WHERE symbol = $1 AND closed_at IS NULL ORDER BY opened_at DESC LIMIT 1
	`, symbol).Scan(
		&t.ID, &t.Symbol, &t.Side, &posSide, &t.EntryPrice, &t.Quantity, &t.StopLoss, &t.TakeProfit,
		&ordID, &t.BalanceBeforeUsdt, &t.BalanceAfterUsdt, &t.OpenedAt, &t.ClosedAt, &t.ExitPrice, &t.RealizedPnl, &closeReason, &t.Leverage, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.PositionSide = strPtr(posSide)
	t.BinanceOrderID = strPtr(ordID)
	t.CloseReason = strPtr(closeReason)
	return &t, nil
}

// ListTrades son N işlemi döner (açık + kapalı).
func (s *Store) ListTrades(ctx context.Context, limit int) ([]Trade, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, symbol, side, COALESCE(position_side, ''), entry_price, quantity, stop_loss, take_profit, COALESCE(binance_order_id, ''), balance_before_usdt, balance_after_usdt, opened_at,
		       closed_at, exit_price, realized_pnl, COALESCE(close_reason, ''), COALESCE(leverage, 1), created_at, updated_at
		FROM trades ORDER BY opened_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Trade
	for rows.Next() {
		var t Trade
		var posSide, ordID, closeReason string
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &posSide, &t.EntryPrice, &t.Quantity, &t.StopLoss, &t.TakeProfit,
			&ordID, &t.BalanceBeforeUsdt, &t.BalanceAfterUsdt, &t.OpenedAt, &t.ClosedAt, &t.ExitPrice, &t.RealizedPnl, &closeReason, &t.Leverage, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.PositionSide = strPtr(posSide)
		t.BinanceOrderID = strPtr(ordID)
		t.CloseReason = strPtr(closeReason)
		list = append(list, t)
	}
	return list, rows.Err()
}

// TotalRealizedPnl kapalı işlemlerin toplam kar/zararı.
func (s *Store) TotalRealizedPnl(ctx context.Context) (float64, error) {
	var total *float64
	err := s.pool.QueryRow(ctx, `SELECT SUM(realized_pnl) FROM trades WHERE closed_at IS NOT NULL`).Scan(&total)
	if err != nil || total == nil {
		return 0, err
	}
	return *total, nil
}

// TradeCounts açık ve kapalı pozisyon sayılarını döner (debug / özet için).
func (s *Store) TradeCounts(ctx context.Context) (open, closed int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE closed_at IS NULL), COUNT(*) FILTER (WHERE closed_at IS NOT NULL) FROM trades`).Scan(&open, &closed)
	return open, closed, err
}
