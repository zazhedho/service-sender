package dto

type SendEmailRequest struct {
	Type           string                 `json:"type" binding:"required,oneof=campaign info notification"`
	To             []string               `json:"to" binding:"required,min=1,dive,email"`
	Subject        string                 `json:"subject" binding:"omitempty,max=200"`
	TextBody       string                 `json:"text_body" binding:"omitempty,max=10000"`
	HTMLBody       string                 `json:"html_body" binding:"omitempty,max=50000"`
	ReplyTo        string                 `json:"reply_to" binding:"omitempty,email"`
	IdempotencyKey string                 `json:"idempotency_key" binding:"omitempty,max=100"`
	TemplateKey    string                 `json:"template_key" binding:"omitempty,max=100"`
	TemplateData   map[string]interface{} `json:"template_data" binding:"omitempty"`
}
