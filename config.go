package resqueExporter

import (
	"os"
	"strconv"
)

type Config struct {
	GuardIntervalMillis int64
	ResqueNamespace     string
	Redis               *RedisConfig
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int64
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func loadConfig() (*Config, error) {
	guardIntervalMillisInt, _ := strconv.Atoi(getEnv("GUARD_INTERVAL_MILLIS", "0"))
	guardIntervalMillis := int64(guardIntervalMillisInt)
	resqueNamespace := getEnv("RESQUE_NAMESPACE", "resque")
	redisHost := os.Getenv("REDIS_HOST")
	redisPort, _ := strconv.Atoi(getEnv("REDIS_PORT", "6379"))
	redisPassword := ""
	redisDB := int64(0)

	config := &Config{
		guardIntervalMillis,
		resqueNamespace,
		&RedisConfig{
			redisHost,
			redisPort,
			redisPassword,
			redisDB,
		},
	}
	return config, nil
}
