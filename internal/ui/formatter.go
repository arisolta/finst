package ui

import (
	"fmt"
	"math"
	"strings"
)

// FormatNumber formats a float with thousand comma separators and specified decimal places.
func FormatNumber(val float64, decimals int) string {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "N/A"
	}

	isNeg := val < 0
	val = math.Abs(val)

	formatStr := fmt.Sprintf("%%.%df", decimals)
	formatted := fmt.Sprintf(formatStr, val)

	parts := strings.Split(formatted, ".")
	intPart := parts[0]
	decPart := ""
	if len(parts) > 1 {
		decPart = "." + parts[1]
	}

	// Insert commas into integer part
	var result []byte
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, intPart[i])
	}

	resStr := string(result) + decPart
	if isNeg {
		return "-" + resStr
	}
	return resStr
}

// FormatCurrencyAmount formats a monetary value in millions (e.g. "20,996.0").
func FormatCurrencyAmount(val float64) string {
	return FormatNumber(val, 1)
}

// FormatPrice formats a share price with 2 decimals (e.g. "$82.40").
func FormatPrice(val float64) string {
	if val <= 0 {
		return "--"
	}
	return "$" + FormatNumber(val, 2)
}

// FormatEPS formats an EPS value with 2 decimals (e.g. "2.72").
func FormatEPS(val float64) string {
	if val == 0 {
		return "0.00"
	}
	return FormatNumber(val, 2)
}

// FormatPercentage formats a percentage with 1 decimal and % sign (e.g. "12.1%").
func FormatPercentage(ptr *float64) string {
	if ptr == nil || math.IsNaN(*ptr) || math.IsInf(*ptr, 0) {
		return "--"
	}
	return fmt.Sprintf("%.1f%%", *ptr)
}

// FormatMultiple formats a valuation multiple with 1 decimal and x (e.g. "44.1x" or "N/A").
func FormatMultiple(ptr *float64) string {
	if ptr == nil || math.IsNaN(*ptr) || math.IsInf(*ptr, 0) || *ptr <= 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1fx", *ptr)
}

// FormatNullableNumber formats a pointer to float64, returning "--" if nil.
func FormatNullableNumber(ptr *float64, decimals int) string {
	if ptr == nil || math.IsNaN(*ptr) || math.IsInf(*ptr, 0) {
		return "--"
	}
	return FormatNumber(*ptr, decimals)
}
