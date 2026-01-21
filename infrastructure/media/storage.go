package media

import (
	"fmt"
	"starter-kit/pkg/logger"
	"starter-kit/pkg/storage"
	"starter-kit/utils"
	"strconv"
	"strings"
)

// InitStorage initializes and returns a storage provider (MinIO or R2)
func InitStorage() (storage.StorageProvider, error) {
	logger.WriteLog(logger.LogLevelDebug, "InitStorage; Initializing storage provider...")

	provider := strings.ToLower(utils.GetEnv("STORAGE_PROVIDER", "minio").(string))

	useSSL, _ := strconv.ParseBool(utils.GetEnv("STORAGE_USE_SSL", "false").(string))

	config := storage.Config{
		Provider:        provider,
		Endpoint:        utils.GetEnv("STORAGE_ENDPOINT", "localhost:9000").(string),
		AccessKeyID:     utils.GetEnv("STORAGE_ACCESS_KEY", "minioadmin").(string),
		SecretAccessKey: utils.GetEnv("STORAGE_SECRET_KEY", "minioadmin").(string),
		BucketName:      utils.GetEnv("STORAGE_BUCKET_NAME", "uploads").(string),
		UseSSL:          useSSL,
		BaseURL:         utils.GetEnv("STORAGE_BASE_URL", "http://localhost:9000").(string),
		Region:          utils.GetEnv("STORAGE_REGION", "auto").(string),
		AccountID:       utils.GetEnv("R2_ACCOUNT_ID", "").(string),
	}

	storageProvider, err := storage.NewStorageProvider(config)
	if err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("InitStorage; Failed to initialize storage provider: %s", err.Error()))
		return nil, fmt.Errorf("failed to initialize storage provider: %w", err)
	}

	logger.WriteLog(logger.LogLevelInfo, fmt.Sprintf("InitStorage; Storage provider initialized successfully. Provider: %s, Endpoint: %s, Bucket: %s",
		provider, config.Endpoint, config.BucketName))

	return storageProvider, nil
}
