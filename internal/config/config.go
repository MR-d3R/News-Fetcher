package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"taskrunner/logger"
	"time"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v2"
)

type Config struct {
	RabbitMQURL  string
	PostgresAddr string
	RedisAddr    string
	QueueName    string
	ServerPort   string
	NewsAPIKey   string
	DB           *redis.Client
}

type YamlConfig struct {
	Server struct {
		Address string `yaml:"addr"`
		Port    string `yaml:"port"`
	} `yaml:"Server"`

	RabbitMQ struct {
		Address   string `yaml:"addr"`
		QueueName string `yaml:"queue_name"`
	} `yaml:"Rabbit"`

	Postgres struct {
		Address string `yaml:"addr"`
		User    string `yaml:"user"`
		DBName  string `yaml:"db_name"`
	} `yaml:"Postgres"`

	Redis struct {
		Address     string `yaml:"addr"`
		Password    string `yaml:"password"`
		User        string `yaml:"user"`
		DB          int    `yaml:"db"`
		MaxRetries  int    `yaml:"max_retries"`
		DialTimeout int    `yaml:"dial_timeout"`
		Timeout     int    `yaml:"timeout"`
	} `yaml:"Redis"`

	NewsAPIKey string `yaml:"NewsAPIKey"`

	Logger struct {
		Level string `yaml:"level"`
	} `yaml:"Logger"`
}

var (
	instance     *Config
	once         sync.Once
	globalLogger *logger.ColorfulLogger
)

func InitConfig(logPrefix string) (*Config, error) {
	var initErr error
	once.Do(func() {
		instance, initErr = initializeConfig(logPrefix)
		// if initErr == nil {
		// 	initErr = instance.InitializeTables()
		// }
	})
	return instance, initErr
}

func GetLogger() *logger.ColorfulLogger {
	return globalLogger
}

func NewClient(ctx context.Context, cfg YamlConfig) (*redis.Client, error) {
	db := redis.NewClient(&redis.Options{
		Addr:        cfg.Redis.Address,
		Password:    cfg.Redis.Password,
		DB:          cfg.Redis.DB,
		Username:    cfg.Redis.User,
		MaxRetries:  cfg.Redis.MaxRetries,
		DialTimeout: time.Duration(cfg.Redis.DialTimeout) * time.Second,
		ReadTimeout: time.Duration(cfg.Redis.Timeout) * time.Second,
	})

	if err := db.Ping(ctx).Err(); err != nil {
		fmt.Printf("failed to connect to redis server: %s\n", err.Error())
		return nil, err
	}

	return db, nil
}

func initializeConfig(logPrefix string) (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to find config.yaml in current path: %v", err)
	}

	var ymlCfg YamlConfig
	err = yaml.Unmarshal(data, &ymlCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %v", err)
	}

	loggerInstance, err := logger.NewColorfulLogger(logPrefix, ymlCfg.Logger.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger: %v", err)
	}

	// Сохраняем логгер как глобальный
	globalLogger = loggerInstance

	// db, err := NewClient(context.Background(), ymlCfg)
	// if err != nil {
	// 	panic(err)
	// }

	var cfg Config
	cfg.RabbitMQURL = ymlCfg.RabbitMQ.Address
	cfg.QueueName = ymlCfg.RabbitMQ.QueueName
	cfg.PostgresAddr = ymlCfg.Postgres.Address
	cfg.RedisAddr = ymlCfg.Redis.Address
	cfg.ServerPort = ymlCfg.Server.Port
	cfg.NewsAPIKey = ymlCfg.NewsAPIKey

	return &cfg, nil
}

func CloseResources() error {
	if instance != nil && instance.DB != nil {
		return instance.DB.Close()
	}
	return nil
}
