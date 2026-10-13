# service-sender

A Go service for sending email through Brevo, exposing MCP tools to chat agents, and optionally providing OTP, password-reset, user-authentication, and RBAC APIs.

## Features

- Send email through the HTTP API or MCP tools using operator-configured senders.
- Use built-in email templates or provide plain-text and HTML content.
- Attach files through MCP as Base64, up to 10 MiB total after decoding.
- Look up Brevo email events through MCP when BREVO_API_KEY is configured.
- Provide OTP, password reset, JWT authentication, Redis sessions, and user/RBAC routes when their backing services are enabled.
- Run the email service without PostgreSQL; database-backed routes are optional.

## Requirements

- Go 1.27 or newer.
- Brevo SMTP credentials for SMTP delivery. MCP can use the Brevo API for sending and tracking.
- PostgreSQL and Redis for the optional database, sessions, OTP, and password-reset features that depend on them.

## Run locally

~~~sh
cp .env.example .env
~~~

Set real local credentials and sender addresses in .env. Do not commit it. For SMTP email, configure SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, and SMTP_FROM. Replace JWT_KEY, OTP_SECRET, and RESET_SECRET before using those features outside local development.

Start the API:

~~~sh
go run .
~~~

The HTTP API listens on PORT (default 8080). MCP is disabled by default. Enable it with MCP_ENABLED=true and configure the MCP settings below. The server loads .env on startup; the example sets APP_ENV=development.

Check that the API is responding:

~~~sh
curl http://localhost:8080/healthcheck
~~~

## Configuration

| Variable | Example/default | Purpose |
| --- | --- | --- |
| APP_NAME | SERVICE-SENDER | Application name used by configuration and logs. |
| APP_ENV | development | Environment name; Compose sets production. |
| PORT | 8080 | HTTP API port. Can also be set with the -port flag. |
| ENABLE_DB | false | Run PostgreSQL migrations and register user/RBAC routes. |
| DATABASE_URL | empty | PostgreSQL DSN; when empty, the DB_* variables are used. |
| DB_HOST, DB_PORT, DB_USERNAME, DB_PASS, DB_NAME | see .env.example | PostgreSQL connection settings when DATABASE_URL is empty. |
| DB_SSLMODE | disable | PostgreSQL TLS mode; configure appropriately for production. |
| PATH_MIGRATE | file://migrations | Migration source used when ENABLE_DB=true. |
| JWT_KEY | placeholder in example | JWT signing key; required to issue access tokens. Use a strong random value. |
| JWT_EXP | 24 | Access-token lifetime in hours. |
| REDIS_URL | empty | Redis URL. If empty, REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, and REDIS_DB are used. |
| LOGIN_ATTEMPT_LIMIT | 5 | Failed login attempts allowed per IP/email pair. |
| LOGIN_ATTEMPT_WINDOW_SECONDS | 60 | Login failure counting window. |
| LOGIN_BLOCK_DURATION_SECONDS | 300 | Lock duration after the login limit is reached. |
| REGISTER_RATE_LIMIT | 5 | Registration requests allowed per IP. |
| REGISTER_RATE_WINDOW_SECONDS | 60 | Registration rate-limit window. |
| OTP_TTL, OTP_TTL_SECONDS | 5m, 300 | OTP lifetime. A valid duration such as 5m takes precedence over seconds. |
| OTP_COOLDOWN, OTP_COOLDOWN_SECONDS | 60s, 60 | Delay before another OTP email can be requested. |
| OTP_MAX_ATTEMPTS | 5 | Maximum OTP verification attempts. |
| OTP_RATE_LIMIT, OTP_RATE_WINDOW | 5, 5m | OTP sends allowed per email and the corresponding window. A _SECONDS fallback is available. |
| OTP_SECRET | placeholder in example | Secret used to hash OTPs; replace it before use. |
| RESET_TTL, RESET_TTL_SECONDS | 15m, 900 | Reset-token lifetime. A valid duration such as 15m takes precedence over seconds. |
| RESET_COOLDOWN, RESET_COOLDOWN_SECONDS | 60s, 60 | Delay before another reset email can be requested. |
| RESET_RATE_LIMIT, RESET_RATE_WINDOW | 5, 15m | Reset emails allowed per email and the corresponding window. A _SECONDS fallback is available. |
| RESET_SECRET | placeholder in example | Secret used to hash reset tokens; replace it before use. |
| RESET_URL_TEMPLATE | example URL | Reset page URL. The {token} placeholder is replaced with the reset token. |
| SMTP_HOST, SMTP_PORT | Brevo, 587 | SMTP host and port. |
| SMTP_USER, SMTP_PASS | apikey, placeholder | Brevo SMTP credentials. Keep secrets in a secret manager or an untracked local .env file. |
| SMTP_FROM | example address | Sender address verified with Brevo. |
| SMTP_SUBJECT, RESET_SUBJECT | see .env.example | Default OTP and password-reset subjects. |
| EMAIL_API_KEY | empty | HTTP email API key, sent by clients in the X-API-Key header. |
| EMAIL_APP_NAME | Account Verification | General email application name; falls back to OTP_APP_NAME. |
| OTP_APP_NAME, RESET_APP_NAME | Account Verification | Application names shown in OTP and password-reset emails. |
| MCP_ENABLED | false | Start the MCP server when true. |
| MCP_ADDR | 127.0.0.1:8081 | MCP bind address; must contain a valid host and port. |
| MCP_SERVER_KEY | empty | Required bearer key when MCP is enabled. Do not include whitespace. |
| EMAIL_VERIFIED_SENDERS | JSON array in .env.example | Sender allowlist. Each entry has id, email, and name. |
| BREVO_API_KEY | empty | Enables Brevo API sending and email-status lookup through MCP. SMTP settings are still required when MCP starts. |
| LOG_LEVEL, LOG_FORMAT | 5, json | Log verbosity and format. |

See [.env.example](.env.example) for all sample values. They are placeholders, not production credentials.

## HTTP API

### Health check

~~~http
GET /healthcheck
~~~

### Send email

POST /api/email/send requires X-API-Key equal to EMAIL_API_KEY. This HTTP endpoint sends through SMTP. File attachments are available through the MCP send_email tool, not this HTTP endpoint.

~~~sh
curl -X POST http://localhost:8080/api/email/send -H 'Content-Type: application/json' -H "X-API-Key: $EMAIL_API_KEY" -d '{"type":"notification","to":["recipient@example.com"],"subject":"Account update","text_body":"There is an update to your account."}'
~~~

The type field is required and must be campaign, info, or notification. Provide text_body or html_body; both may be provided. Limits: subject 200 characters, text 10,000 characters, and HTML 50,000 characters. Optional fields are reply_to, template_key, template_data, and idempotency_key. The idempotency key is a correlation value only; it does not prevent duplicate email.

Built-in templates:

- campaign_default: Headline, Message, CTAURL, and CTALabel.
- info_default: Title, Message, and Reference.
- notification_default: Title, Message, Timestamp, ActionURL, and ActionLabel.

All templates also accept Subject and AppName. Explicit text_body or html_body replaces the rendered content for that format.

A successful response means the SMTP server accepted the message. It does not confirm inbox delivery or that a person read it.

### OTP and password reset

| Method | Path | Purpose |
| --- | --- | --- |
| POST | /api/auth/otp/send | Send a registration OTP; requires Redis. |
| POST | /api/auth/otp/verify | Verify a registration OTP; requires Redis. |
| POST | /api/auth/reset-password/email | Send a reset email with the EMAIL_API_KEY. |
| POST | /api/auth/reset-password/request | Create a reset request; requires Redis. |
| POST | /api/auth/reset-password/verify | Verify a reset token; requires Redis. |

## User and administration routes

These routes are registered only when ENABLE_DB=true. Private routes require Authorization: Bearer <access-token>. Administration operations also check the user's permissions.

| Method | Path | Access |
| --- | --- | --- |
| POST | /api/user/register | Public; rate limiting uses Redis. |
| POST | /api/user/login | Public. |
| POST | /api/user/forgot-password | Public. |
| POST | /api/user/reset-password | Public. |
| GET | /api/users | JWT and users:list permission. |
| GET, PUT, DELETE | /api/user | JWT; current user's profile. |
| GET, PUT, DELETE | /api/user/:id | JWT and permission for the operation. |
| PUT | /api/user/change/password | JWT. |
| POST | /api/user | JWT and users:create permission; admin creates a user. |
| POST, GET, PUT, DELETE | /api/role and /api/role/:id | JWT and role permissions; includes permission/menu assignment. |
| GET | /api/roles | JWT and roles:list permission. |
| POST, GET, PUT, DELETE | /api/permission and /api/permission/:id | JWT and permission permissions. |
| GET | /api/permissions | JWT and permissions:list permission. |
| GET | /api/permissions/me | JWT; current user's permissions. |
| POST, GET, PUT, DELETE | /api/menu and /api/menu/:id | JWT and menu permissions. |
| GET | /api/menus | JWT and menus:list permission. |
| GET | /api/menus/active, /api/menus/me | JWT. |
| POST | /api/user/logout | JWT. |
| GET | /api/user/sessions | JWT; requires Redis. |
| DELETE | /api/user/session/:session_id | JWT; requires Redis. |
| POST | /api/user/sessions/revoke-others | JWT; requires Redis. |

## MCP server for chat agents

Set MCP_ENABLED=true, then configure MCP_SERVER_KEY and EMAIL_VERIFIED_SENDERS. Each allowlisted address must already be verified by the operator in Brevo.

~~~dotenv
MCP_ENABLED=true
MCP_ADDR=127.0.0.1:8081
MCP_SERVER_KEY=replace-with-a-long-random-key
EMAIL_VERIFIED_SENDERS='[{"id":"support","email":"support@example.com","name":"Support"}]'
~~~

The MCP endpoint is /mcp and uses this header:

~~~http
Authorization: Bearer <MCP_SERVER_KEY>
~~~

Available tools:

- list_email_senders: list the configured sender allowlist.
- send_email: send to recipients using a sender_id, with a subject, text/HTML body, and optional attachments.
- get_email_status: available only when BREVO_API_KEY is set; looks up events with the sender_id and message_id returned by send_email.

The send_email type can be campaign, info, or notification; it defaults to notification. Attachments use standard Base64 content in content_base64 and a filename in name. Filenames cannot contain paths. The combined decoded attachment size is limited to 10 MiB; each attachment must be valid, non-empty Base64. The MCP request-body limit is 15 MiB.

For one recipient, the result includes message_id when tracking is available. For multiple recipients, use each message_id in the tracking array to look up that recipient. A failure after some messages were sent can return partial_or_unknown. Do not retry automatically after an error or an unknown result. idempotency_key does not deduplicate sends.

A delivered event means the recipient's mail server accepted the message. An opened event is a tracking signal, not proof that a person read the email. Brevo status queries cover the last 90 days.

Without BREVO_API_KEY, MCP uses SMTP and does not return Brevo message IDs or expose get_email_status. BREVO_API_KEY changes the MCP email path only; the HTTP email API still uses SMTP.

## PostgreSQL and Redis

ENABLE_DB=true runs migrations from PATH_MIGRATE at startup and registers user/RBAC routes. DATABASE_URL takes precedence over the individual DB_* variables.

Configure Redis with REDIS_URL or REDIS_HOST/REDIS_PORT, plus an optional password and database number. Redis supports rate limiting, OTP, password reset, and sessions. Session routes are registered only when both database routes and Redis are available. Keep Redis healthy when enabling features that depend on it.

## Tests and local checks

~~~sh
go test ./...
go vet ./...
gofmt -l $(rg --files -g '*.go')
~~~

gofmt -l does not change files; no output means all Go files are formatted.

## Docker and deployment

Compose uses the GHCR image and reads .env from the deployment directory:

~~~sh
docker compose pull
docker compose up -d
docker compose logs -f service_sender
~~~

The checked-in Compose file maps API container port 8080 to 127.0.0.1:8088 and MCP port 8081 to 127.0.0.1:8089. A host reverse proxy can forward the required traffic. Do not expose MCP publicly without authentication and network restrictions.

The Docker image runs as a non-root user. A push of application changes to main triggers GitHub Actions to build and publish the image, then run docker compose pull and docker compose up -d in /opt/apps/service-sender. The workflow needs the GitHub Production environment and secrets VPS_HOST, VPS_USER, and VPS_SSH_KEY. README-only changes are excluded from that workflow.

## Code layout

- internal/handlers/http: HTTP API handlers.
- internal/handlers/mcp: MCP server and email tools.
- internal/services: email, OTP, reset, user, RBAC, and session application logic.
- internal/repositories: PostgreSQL/Redis access.
- pkg/mailer: SMTP, Brevo API, MIME, attachments, and email-event tracking.
- pkg/config: MCP, OTP, password-reset, and application configuration.
- migrations: PostgreSQL migrations.

