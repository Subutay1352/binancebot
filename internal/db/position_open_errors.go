package db

import (
	"context"
	"log"
	"time"
)

// PositionOpenError pozisyon açmaya çalışırken alınan hata kaydı.
type PositionOpenError struct {
	ID           int64     `json:"id"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"` // LONG, SHORT
	ErrorMessage string    `json:"error_message"`
	InstanceID   *string   `json:"instance_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// InsertPositionOpenError pozisyon açma hatasını tabloya yazar. instanceID boş olabilir.
func (s *Store) InsertPositionOpenError(ctx context.Context, symbol, side, errorMessage string, instanceID *string) (int64, error) {
	inst := ""
	if instanceID != nil {
		inst = *instanceID
	}
	var id int64
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO position_open_errors (symbol, side, error_message, instance_id)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, created_at
	`, symbol, side, errorMessage, inst).Scan(&id, &createdAt)
	if err != nil {
		return 0, err
	}
	log.Printf("[db] position_open_error | id=%d symbol=%s side=%s err=%s", id, symbol, side, errorMessage)
	return id, nil
}
