package strategy

// Signal stratejinin verdiği yön. Sonradan kuralları değiştirince aynı tip kullanılır.
type Signal string

const (
	Hold  Signal = "HOLD"
	Long  Signal = "LONG"
	Short Signal = "SHORT"
)
