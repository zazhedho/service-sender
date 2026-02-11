package serviceemail

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

var ErrTemplateNotFound = fmt.Errorf("email template not found")
var ErrSubjectRequired = fmt.Errorf("subject is required")

func renderEmailContent(reqSubject, reqText, reqHTML, templateKey string, templateData map[string]interface{}, appName string) (string, string, string, error) {
	subject := strings.TrimSpace(reqSubject)
	textBody := strings.TrimSpace(reqText)
	htmlBody := strings.TrimSpace(reqHTML)

	key := strings.TrimSpace(templateKey)
	if key == "" {
		if subject == "" {
			return "", "", "", ErrSubjectRequired
		}
		if textBody == "" && htmlBody == "" {
			return "", "", "", ErrEmailBodyRequired
		}
		return subject, textBody, htmlBody, nil
	}

	tpl, ok := defaultTemplates[key]
	if !ok {
		return "", "", "", ErrTemplateNotFound
	}

	data := map[string]interface{}{
		"AppName": appName,
		"Subject": subject,
	}
	for k, v := range templateData {
		data[k] = v
	}

	renderedSubject, err := renderTextTemplate("email_subject", tpl.Subject, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render subject: %w", err)
	}
	renderedText, err := renderTextTemplate("email_text", tpl.Text, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	renderedHTML, err := renderHTMLTemplate("email_html", tpl.HTML, data)
	if err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}

	if strings.TrimSpace(textBody) != "" {
		renderedText = textBody
	}
	if strings.TrimSpace(htmlBody) != "" {
		renderedHTML = htmlBody
	}

	return strings.TrimSpace(renderedSubject), strings.TrimSpace(renderedText), strings.TrimSpace(renderedHTML), nil
}

func renderTextTemplate(name, content string, data map[string]interface{}) (string, error) {
	tpl, err := texttemplate.New(name).Parse(content)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderHTMLTemplate(name, content string, data map[string]interface{}) (string, error) {
	tpl, err := htmltemplate.New(name).Parse(content)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
