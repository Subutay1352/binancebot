package risk

// Prices entry fiyatı ve yüzdelere göre stop loss / take profit fiyatlarını döner (ham; yuvarlama bot'ta tick size ile yapılır).
// side: "LONG" veya "SHORT"
func Prices(entryPrice, stopLossPercent, takeProfitPercent float64, side string) (stopLoss, takeProfit float64) {
	slPct := stopLossPercent / 100
	tpPct := takeProfitPercent / 100
	if side == "LONG" {
		stopLoss = entryPrice * (1 - slPct)
		takeProfit = entryPrice * (1 + tpPct)
	} else {
		stopLoss = entryPrice * (1 + slPct)
		takeProfit = entryPrice * (1 - tpPct)
	}
	return stopLoss, takeProfit
}
