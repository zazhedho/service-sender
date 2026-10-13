package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"service-sender/pkg/logger"
	"service-sender/utils"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func GetAppConf(key string, def any, rdbCache *redis.Client) any {
	cacheKey := utils.RedisAppConf
	appConf := make(map[string]string)
	var cache, getNewConfig bool
	if consul := os.Getenv("CONSUL"); consul != "" {
		appConf, cache, getNewConfig = loadConsulAppConf(consul, cacheKey, rdbCache)
	} else {
		appConf = loadLocalAppConf()
	}

	applyAppConf(appConf, cache && getNewConfig, cacheKey, rdbCache)
	return utils.GetEnv(key, def)
}

func loadConsulAppConf(consul, cacheKey string, rdbCache *redis.Client) (map[string]string, bool, bool) {
	appConf := make(map[string]string)
	cache := strings.ToLower(os.Getenv("CACHE")) == "on" && rdbCache != nil
	getNewConfig := !cache
	if cache {
		appConf, getNewConfig = readCachedAppConf(rdbCache, cacheKey)
	}
	if getNewConfig {
		appConf = fetchConsulAppConf(consul, appConf)
	}
	return appConf, cache, getNewConfig
}

func readCachedAppConf(rdbCache *redis.Client, cacheKey string) (map[string]string, bool) {
	appConf := make(map[string]string)
	jsonAppConf, err := rdbCache.Get(context.Background(), cacheKey).Result()
	if err != nil {
		return appConf, errors.Is(err, redis.Nil)
	}
	if err = json.Unmarshal([]byte(jsonAppConf), &appConf); err != nil {
		logger.WriteLog(logger.LogLevelError, "utils.GetAppConf; invalid cached application configuration; loading from Consul")
		return appConf, true
	}
	return appConf, false
}

func fetchConsulAppConf(consul string, appConf map[string]string) map[string]string {
	consulPath := fmt.Sprintf("%s/%s", os.Getenv("CONSUL_PATH"), os.Getenv("APP_ENV"))
	runtimeViper := viper.New()
	runtimeViper.AddRemoteProvider("consul", consul, consulPath)
	runtimeViper.SetConfigType("json")
	if err := runtimeViper.ReadRemoteConfig(); err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("utils.GetAppConf; Loading config: %s/%s; error: %+v;", consul, consulPath, err))
	} else if err := runtimeViper.Unmarshal(&appConf); err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("utils.GetAppConf; Loading congif: %s/%s; error: unable to decode into map, %+v;", consul, consulPath, err))
	}
	return appConf
}

func loadLocalAppConf() map[string]string {
	appConf := make(map[string]string)
	configName := "app"
	pathConfig := os.Getenv("APP_CONFIG")
	if pathConfig == "" {
		pathConfig = "config"
		configName = os.Getenv("APP_ENV")
	}

	viper.AddConfigPath(pathConfig)
	viper.SetConfigType("env")
	viper.SetConfigName(configName)

	if err := viper.ReadInConfig(); err != nil {
		logger.WriteLog(logger.LogLevelError, fmt.Sprintf("utils.GetAppConf; Loading config: %s - %s.env; error:  %+v;", pathConfig, configName, err))
	} else {
		_ = viper.Unmarshal(&appConf)
	}
	return appConf
}

func applyAppConf(appConf map[string]string, cacheConfig bool, cacheKey string, rdbCache *redis.Client) {
	if len(appConf) > 0 {
		if _, ok := appConf["config_id"]; !ok {
			appConf["config_id"] = uuid.NewString()
		}

		if appConf["config_id"] != os.Getenv("CONFIG_ID") {
			for k, v := range appConf {
				os.Setenv(strings.ToUpper(k), v)
			}
		}

		if cacheConfig {
			go func(rdbCache *redis.Client, cacheKey string, data map[string]string) {
				if cacheData, err := json.Marshal(data); err == nil {
					ttl := utils.GetEnv("TTL_CACHE_CONFIG_APP", time.Duration(60*60*24)).(time.Duration) * time.Second
					_ = rdbCache.Set(context.Background(), cacheKey, string(cacheData), ttl).Err()
				}
			}(rdbCache, cacheKey, appConf)
		}
	}
}
