package database

import (
	"database/sql"
	"fmt"
	"starter-kit/pkg/logger"
	"starter-kit/utils"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnDb() (db *gorm.DB, sqlDB *sql.DB, err error) {
	dsn := utils.GetEnv("DATABASE_URL", "").(string)

	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Jakarta",
			utils.GetEnv("DB_HOST", "").(string),
			utils.GetEnv("DB_PORT", "").(string),
			utils.GetEnv("DB_USERNAME", "").(string),
			utils.GetEnv("DB_PASS", "").(string),
			utils.GetEnv("DB_NAME", "").(string),
			utils.GetEnv("DB_SSLMODE", "disable").(string))
	}
	logger.WriteLog(logger.LogLevelDebug, fmt.Sprintf("ConnDb; Initialize db connection..."))

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("ConnDb; %s Error: %s", dsn, err.Error()))
		return
	}

	maxIdle := 10
	maxIdleTime := 5 * time.Minute
	maxConn := 100
	maxLifeTime := time.Hour

	sqlDB, err = db.DB()
	if err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("ConnDb.sqlDB; %s Error: %s", dsn, err.Error()))
		return
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxIdleTime(maxIdleTime)
	sqlDB.SetMaxOpenConns(maxConn)
	sqlDB.SetConnMaxLifetime(maxLifeTime)

	db.Debug()

	return
}
