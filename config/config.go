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
	RedisSt struct {
		Address     string `yaml:"addr"`
		Password    string `yaml:"password"`
		User        string `yaml:"user"`
		DB          int    `yaml:"db"`
		MaxRetries  int    `yaml:"max_retries"`
		DialTimeout int    `yaml:"dial_timeout"`
		Timeout     int    `yaml:"timeout"`
	} `yaml:"Redis"`

	LoggerSt struct {
		Level string `yaml:"level"`
	} `yaml:"Logger"`

	DB     *redis.Client
	Logger *logger.ColorfulLogger
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

func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	db := redis.NewClient(&redis.Options{
		Addr:        cfg.RedisSt.Address,
		Password:    cfg.RedisSt.Password,
		DB:          cfg.RedisSt.DB,
		Username:    cfg.RedisSt.User,
		MaxRetries:  cfg.RedisSt.MaxRetries,
		DialTimeout: time.Duration(cfg.RedisSt.DialTimeout) * time.Second,
		ReadTimeout: time.Duration(cfg.RedisSt.Timeout) * time.Second,
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

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %v", err)
	}

	loggerInstance, err := logger.NewColorfulLogger(logPrefix, cfg.LoggerSt.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger: %v", err)
	}

	// Сохраняем логгер как глобальный
	globalLogger = loggerInstance

	db, err := NewClient(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	cfg.DB = db
	return &cfg, nil
}

func CloseResources() error {
	if instance != nil && instance.DB != nil {
		return instance.DB.Close()
	}
	return nil
}
