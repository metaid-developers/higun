package api

import (
	"strconv"
	"time"
)

func short(s interface{}) string {
	var str string
	switch v := s.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return ""
	}
	r := []rune(str)
	if len(r) <= 10 {
		return str
	}
	return string(r[:5]) + "..." + string(r[len(r)-5:])
}

// Custom date formatting function, supports string and int64 types, with debug information
func formatDate(layout string, timestamp interface{}) string {
	var ts int64
	switch v := timestamp.(type) {
	case int64:
		ts = v
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return "Invalid timestamp"
		}
		ts = parsed
	default:
		return "Invalid timestamp"
	}
	return time.Unix(ts, 0).Format(layout)
}
func add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}
