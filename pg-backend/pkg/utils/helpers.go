package utils

import (
	"fmt"
	"strconv"
)

func ConvertToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func MustParseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func MustParseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
