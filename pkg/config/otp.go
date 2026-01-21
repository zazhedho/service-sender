package config

import (
	"strings"
	"time"

	"service-otp/utils"
)

type OTPConfig struct {
	TTL         time.Duration
	MaxAttempts int
	RateLimit   int
	RateWindow  time.Duration
	Cooldown    time.Duration
	Secret      string
}

func LoadOTPConfig() OTPConfig {
	ttl := time.Duration(utils.GetEnv("OTP_TTL_SECONDS", 300).(int)) * time.Second
	if v := strings.TrimSpace(utils.GetEnv("OTP_TTL", "").(string)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}

	cooldown := time.Duration(utils.GetEnv("OTP_COOLDOWN_SECONDS", 60).(int)) * time.Second
	if v := strings.TrimSpace(utils.GetEnv("OTP_COOLDOWN", "").(string)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cooldown = d
		}
	}

	rateWindow := time.Duration(utils.GetEnv("OTP_RATE_WINDOW_SECONDS", int(ttl.Seconds())).(int)) * time.Second
	if v := strings.TrimSpace(utils.GetEnv("OTP_RATE_WINDOW", "").(string)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			rateWindow = d
		}
	}

	maxAttempts := utils.GetEnv("OTP_MAX_ATTEMPTS", 5).(int)
	rateLimit := utils.GetEnv("OTP_RATE_LIMIT", 5).(int)

	secret := strings.TrimSpace(utils.GetEnv("OTP_SECRET", "otp-secret").(string))

	return OTPConfig{
		TTL:         ttl,
		MaxAttempts: maxAttempts,
		RateLimit:   rateLimit,
		RateWindow:  rateWindow,
		Cooldown:    cooldown,
		Secret:      secret,
	}
}
