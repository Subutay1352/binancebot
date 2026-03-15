package binance

import "strconv"

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func formatQty(qty float64) string {
	return strconv.FormatFloat(qty, 'f', -1, 64)
}
