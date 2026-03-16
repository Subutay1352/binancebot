package binance

import (
	"fmt"
	"strconv"
	"strings"
)

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func formatQty(qty float64) string {
	return strconv.FormatFloat(qty, 'f', -1, 64)
}

// FormatQuantityForAPI Binance'ın kabul ettiği miktar string'i (step size hassasiyeti). Pozisyon kapatırken kullanılır.
func FormatQuantityForAPI(qty float64, stepSizeStr string) string {
	step := parseFloat(stepSizeStr)
	if step <= 0 {
		return fmt.Sprintf("%.8f", qty)
	}
	qty = roundToStep(qty, step)
	decimals := tickDecimals(step)
	s := fmt.Sprintf("%.*f", decimals, qty)
	if strings.Contains(s, ".") {
		for len(s) > 1 && s[len(s)-1] == '0' {
			s = s[:len(s)-1]
		}
		if len(s) > 0 && s[len(s)-1] == '.' {
			s = s[:len(s)-1]
		}
	}
	return s
}

func roundToStep(qty float64, step float64) float64 {
	if step <= 0 {
		return qty
	}
	inv := 1.0 / step
	return float64(int64(qty*inv)) * step
}

func tickDecimals(tick float64) int {
	if tick >= 1 || tick <= 0 {
		return 2
	}
	d := 0
	for tick < 1 {
		d++
		tick *= 10
	}
	return d
}
