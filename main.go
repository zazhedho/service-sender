package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"service-sender/infrastructure/database"
	mcpHandler "service-sender/internal/handlers/mcp"
	"service-sender/internal/router"
	"service-sender/pkg/config"
	"service-sender/pkg/logger"
	"service-sender/pkg/mailer"
	"service-sender/utils"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // Register the PostgreSQL migration driver.
	_ "github.com/golang-migrate/migrate/v4/source/file"       // Register the file-based migration source.
	"github.com/joho/godotenv"
)

func FailOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: initialization failed", msg)
	}
}

func main() {
	var (
		err   error
		sqlDb *sql.DB
	)
	configureServerEnvironment()
	mcpConfig, err := config.LoadMCPConfig()
	FailOnError(err, "Invalid MCP configuration")

	var port, appName string
	flag.StringVar(&port, "port", os.Getenv("PORT"), "port of the service")
	flag.StringVar(&appName, "appname", os.Getenv("APP_NAME"), "service name")
	flag.Parse()
	logger.WriteLog(logger.LogLevelInfo, "APP: "+appName+"; PORT: "+port)

	confID := config.GetAppConf("CONFIG_ID", "", nil)
	logger.WriteLog(logger.LogLevelDebug, fmt.Sprintf("ConfigID: %s", confID))

	dbEnabled := utils.GetEnv("ENABLE_DB", false).(bool)
	if dbEnabled {
		runMigration()
	} else {
		logger.WriteLog(logger.LogLevelInfo, "DB disabled; skipping migrations and DB-backed routes")
	}

	// Initialize Redis for session management (optional)
	redisClient, err := database.InitRedis()
	if err != nil {
		logger.WriteLog(logger.LogLevelDebug, "Redis not available, session management will be disabled")
	} else {
		defer database.CloseRedis()
		logger.WriteLog(logger.LogLevelInfo, "Redis initialized, session management enabled")
	}

	routes := router.NewRoutes()

	if dbEnabled {
		routes.DB, sqlDb, err = database.ConnDb()
		FailOnError(err, "Failed to open db")
		defer sqlDb.Close()

		routes.UserRoutes()
		routes.RoleRoutes()
		routes.PermissionRoutes()
		routes.MenuRoutes()

		// Register session routes if Redis is available
		if redisClient != nil {
			routes.SessionRoutes()
		}
	}

	routes.OTPRoutes()
	routes.PasswordResetRoutes()
	routes.EmailRoutes()

	logger.WriteLog(logger.LogLevelInfo, "All routes registered successfully")

	var mcpServer *http.Server
	if mcpConfig.Enabled {
		sender, senderErr := mailer.NewBrevoSenderFromEnv()
		FailOnError(senderErr, "Failed to configure MCP email sender")
		emailAppName, nameErr := mcpEmailAppName()
		FailOnError(nameErr, "Invalid MCP email app name")
		mcpServer = &http.Server{
			Addr:              mcpConfig.Addr,
			Handler:           mcpHandler.NewHTTPHandler(mcpConfig, mcpEmailServices(mcpConfig, *sender), emailAppName),
			ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
			ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second,
		}
	}
	serverContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	err = serveHTTPServers(serverContext, &http.Server{Addr: fmt.Sprintf(":%s", port), Handler: routes.App, ReadHeaderTimeout: 10 * time.Second}, mcpServer)
	FailOnError(err, "Failed run service")
}

func configureServerEnvironment() {
	if timeZone, err := time.LoadLocation("Asia/Jakarta"); err != nil {
		logger.WriteLog(logger.LogLevelError, "time.LoadLocation - Error: "+err.Error())
	} else {
		time.Local = timeZone
	}

	if err := godotenv.Load(".env"); err != nil && os.Getenv("APP_ENV") == "" {
		log.Fatalf("Error app environment")
	}

	myAddr := "unknown"
	addrs, _ := net.InterfaceAddrs()
	for _, address := range addrs {
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				myAddr = ipNet.IP.String()
				break
			}
		}
	}

	myAddr += strings.Repeat(" ", 15-len(myAddr))
	os.Setenv("ServerIP", myAddr)
	logger.WriteLog(logger.LogLevelInfo, "Server IP: "+myAddr)
}

func runMigration() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			utils.GetEnv("DB_USERNAME", "").(string),
			utils.GetEnv("DB_PASS", "").(string),
			utils.GetEnv("DB_HOST", "").(string),
			utils.GetEnv("DB_PORT", "").(string),
			utils.GetEnv("DB_NAME", "").(string),
			utils.GetEnv("DB_SSLMODE", "disable").(string))
	}

	m, err := migrate.New(utils.GetEnv("PATH_MIGRATE", "file://migrations").(string), dsn)
	if err != nil {
		log.Fatal("failed to initialize database migration")
	}

	if err := m.Up(); err != nil && err.Error() != "no change" {
		log.Fatal("database migration failed")
	}
	logger.WriteLog(logger.LogLevelInfo, "Migration Success")
}
