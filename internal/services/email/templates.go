package serviceemail

type emailTemplate struct {
	Subject string
	Text    string
	HTML    string
}

var defaultTemplates = map[string]emailTemplate{
	"campaign_default": {
		Subject: "{{if .Subject}}{{.Subject}}{{else}}Campaign Update{{end}}",
		Text: `Halo,

{{if .Headline}}{{.Headline}}{{else}}Campaign Update{{end}}

{{if .Message}}{{.Message}}{{else}}Kami punya update terbaru untuk Anda.{{end}}
{{if .CTAURL}}
Lihat selengkapnya: {{.CTAURL}}
{{end}}
Terima kasih,
{{.AppName}} Team`,
		HTML: `<!DOCTYPE html>
<html lang="id">
<body style="margin:0;padding:24px;background:#f3f6fb;font-family:Segoe UI,Tahoma,Arial,sans-serif;color:#1f2937;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:640px;margin:0 auto;background:#ffffff;border-radius:14px;overflow:hidden;border:1px solid #e5e7eb;">
    <tr>
      <td style="background:linear-gradient(120deg,#0f172a,#1d4ed8);padding:28px 24px;">
        <p style="margin:0;color:#bfdbfe;font-size:12px;letter-spacing:1px;text-transform:uppercase;">Campaign</p>
        <h1 style="margin:8px 0 0 0;color:#ffffff;font-size:24px;line-height:1.3;">{{if .Headline}}{{.Headline}}{{else}}Campaign Update{{end}}</h1>
      </td>
    </tr>
    <tr>
      <td style="padding:24px;">
        <p style="margin:0 0 16px 0;color:#475569;font-size:15px;line-height:1.7;">{{if .Message}}{{.Message}}{{else}}Kami punya update terbaru untuk Anda.{{end}}</p>
        {{if .CTAURL}}
        <p style="margin:0 0 20px 0;">
          <a href="{{.CTAURL}}" style="display:inline-block;padding:12px 18px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;">{{if .CTALabel}}{{.CTALabel}}{{else}}Lihat Detail{{end}}</a>
        </p>
        <p style="margin:0;color:#64748b;font-size:12px;line-height:1.6;">Jika tombol tidak berfungsi, buka link ini: <a href="{{.CTAURL}}" style="color:#2563eb;word-break:break-all;">{{.CTAURL}}</a></p>
        {{end}}
      </td>
    </tr>
    <tr>
      <td style="padding:16px 24px;background:#f8fafc;border-top:1px solid #e5e7eb;">
        <p style="margin:0;color:#94a3b8;font-size:12px;">Dikirim oleh {{.AppName}}</p>
      </td>
    </tr>
  </table>
</body>
</html>`,
	},
	"info_default": {
		Subject: "{{if .Subject}}{{.Subject}}{{else}}Informasi Penting{{end}}",
		Text: `Halo,

{{if .Title}}{{.Title}}{{else}}Informasi Penting{{end}}

{{if .Message}}{{.Message}}{{else}}Ada informasi penting untuk Anda.{{end}}
{{if .Reference}}Referensi: {{.Reference}}{{end}}

Salam,
{{.AppName}} Team`,
		HTML: `<!DOCTYPE html>
<html lang="id">
<body style="margin:0;padding:24px;background:#f8fafc;font-family:Segoe UI,Tahoma,Arial,sans-serif;color:#1e293b;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:640px;margin:0 auto;background:#ffffff;border-radius:14px;overflow:hidden;border:1px solid #e2e8f0;">
    <tr>
      <td style="padding:22px 24px;background:#0f766e;">
        <p style="margin:0;color:#ccfbf1;font-size:12px;letter-spacing:1px;text-transform:uppercase;">Informasi</p>
        <h2 style="margin:8px 0 0 0;color:#ffffff;font-size:22px;">{{if .Title}}{{.Title}}{{else}}Informasi Penting{{end}}</h2>
      </td>
    </tr>
    <tr>
      <td style="padding:24px;">
        <div style="background:#f8fafc;border-left:4px solid #14b8a6;border-radius:8px;padding:14px 16px;margin-bottom:16px;">
          <p style="margin:0;color:#334155;font-size:15px;line-height:1.7;">{{if .Message}}{{.Message}}{{else}}Ada informasi penting untuk Anda.{{end}}</p>
        </div>
        {{if .Reference}}
        <p style="margin:0;color:#64748b;font-size:13px;">Referensi: <strong style="color:#334155;">{{.Reference}}</strong></p>
        {{end}}
      </td>
    </tr>
    <tr>
      <td style="padding:16px 24px;background:#f8fafc;border-top:1px solid #e2e8f0;">
        <p style="margin:0;color:#94a3b8;font-size:12px;">{{.AppName}}</p>
      </td>
    </tr>
  </table>
</body>
</html>`,
	},
	"notification_default": {
		Subject: "{{if .Subject}}{{.Subject}}{{else}}Notifikasi{{end}}",
		Text: `Halo,

{{if .Message}}{{.Message}}{{else}}Anda menerima notifikasi baru.{{end}}
{{if .ActionURL}}
Tindak lanjuti di: {{.ActionURL}}
{{end}}
Waktu: {{if .Timestamp}}{{.Timestamp}}{{else}}Sekarang{{end}}

Salam,
{{.AppName}}`,
		HTML: `<!DOCTYPE html>
<html lang="id">
<body style="margin:0;padding:24px;background:#f1f5f9;font-family:Segoe UI,Tahoma,Arial,sans-serif;color:#0f172a;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:620px;margin:0 auto;background:#ffffff;border-radius:12px;overflow:hidden;border:1px solid #e2e8f0;">
    <tr>
      <td style="padding:20px 24px;background:#1e293b;">
        <p style="margin:0;color:#cbd5e1;font-size:12px;letter-spacing:1px;text-transform:uppercase;">Notification</p>
        <h3 style="margin:8px 0 0 0;color:#ffffff;font-size:20px;">{{if .Title}}{{.Title}}{{else}}Notifikasi Baru{{end}}</h3>
      </td>
    </tr>
    <tr>
      <td style="padding:24px;">
        <p style="margin:0 0 14px 0;color:#334155;font-size:15px;line-height:1.7;">{{if .Message}}{{.Message}}{{else}}Anda menerima notifikasi baru.{{end}}</p>
        <p style="margin:0 0 18px 0;color:#64748b;font-size:12px;">Waktu: <strong style="color:#334155;">{{if .Timestamp}}{{.Timestamp}}{{else}}Sekarang{{end}}</strong></p>
        {{if .ActionURL}}
        <a href="{{.ActionURL}}" style="display:inline-block;padding:10px 16px;background:#0ea5e9;color:#ffffff;text-decoration:none;border-radius:8px;font-weight:600;">{{if .ActionLabel}}{{.ActionLabel}}{{else}}Lihat Detail{{end}}</a>
        {{end}}
      </td>
    </tr>
  </table>
</body>
</html>`,
	},
}
