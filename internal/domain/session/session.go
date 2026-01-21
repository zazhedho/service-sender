package domainsession

import "time"

type Session struct {
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	Token        string    `json:"token"`
	DeviceInfo   string    `json:"device_info,omitempty"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	LoginAt      time.Time `json:"login_at"`
	LastActivity time.Time `json:"last_activity"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type SessionInfo struct {
	SessionID        string    `json:"session_id"`
	DeviceInfo       string    `json:"device_info"`
	IP               string    `json:"ip"`
	LoginAt          time.Time `json:"login_at"`
	LastActivity     time.Time `json:"last_activity"`
	IsCurrentSession bool      `json:"is_current_session"`
}
