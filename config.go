package resqueExporter

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strconv"
)

type Config struct {
	GuardIntervalMillis int64
	ResqueNamespace     string
	Redis               *RedisConfig
	QueueConfiguration  *QueueConfiguration
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int64
}

type QueueConfiguration struct {
	QueueToWorkers map[string][]string `yaml:queue_to_workers`
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getQueueConfiguration(filePath string) (config *QueueConfiguration) {
	yamlFile, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}

	return config
}

func loadConfig() (*Config, error) {
	guardIntervalMillisInt, _ := strconv.Atoi(getEnv("GUARD_INTERVAL_MILLIS", "0"))
	guardIntervalMillis := int64(guardIntervalMillisInt)
	resqueNamespace := getEnv("RESQUE_NAMESPACE", "resque")
	redisHost := os.Getenv("REDIS_HOST")
	redisPort, _ := strconv.Atoi(getEnv("REDIS_PORT", "6379"))
	redisPassword := ""
	redisDB := int64(0)
	queueConfigurationFilePath := getEnv("QUEUE_CONFIGURATION_FILE_PATH", "./config.yml")
	queueConfiguration := getQueueConfiguration(queueConfigurationFilePath)

	config := &Config{
		GuardIntervalMillis: guardIntervalMillis,
		ResqueNamespace:     resqueNamespace,
		Redis: &RedisConfig{
			redisHost,
			redisPort,
			redisPassword,
			redisDB,
		},
		QueueConfiguration: queueConfiguration,
	}
	return config, nil
}
