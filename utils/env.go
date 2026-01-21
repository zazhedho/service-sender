package utils

import (
	"os"
	"strconv"
	"time"
)

func GetEnv(key string, def interface{}) interface{} {
	v, isset := os.LookupEnv(key)
	if !isset {
		return def
	}

	switch def.(type) {
	case int:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
		break
	case time.Duration:
		if v == "eod" {
			eodTime, _ := time.Parse(time.RFC3339, time.Now().Format("2006-01-02")+"T23:59:59+07:00")
			return time.Duration(int(eodTime.Sub(time.Now()).Seconds()))
		}

		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			return time.Duration(i)
		} else {
			return time.Minute
		}
	case bool:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		break
	case string:
		return v
	}
	return def
}
