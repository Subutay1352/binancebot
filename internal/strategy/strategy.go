package strategy

import "context"

// Strategy arayüzü: piyasa verisine göre Long / Short / Hold kararı.
// Farklı kurallar için yeni implementasyon yazılabilir.
type Strategy interface {
	Decide(ctx context.Context, symbol string) (Signal, error)
}
