package config

import (
	"fmt"

	"github.com/caarlos0/env/v6"
	"github.com/spf13/pflag"
)

const (
	defaultRunAddress    = ":8080"
	defaultDatabaseURI   = "postgres://gophprofile:gophprofile@localhost:5432/gophprofile?sslmode=disable"
	defaultLogLevel      = "info"
	defaultStaticDir     = "web/static"
	defaultMaxUploadSize = 10 << 20
	defaultS3Endpoint    = "localhost:9000"
	defaultS3AccessKey   = "minioadmin"
	defaultS3SecretKey   = "minioadmin"
	defaultS3Bucket      = "avatars"
	defaultKafkaTopic    = "avatar-events"
	defaultKafkaGroupID  = "avatar-worker"
	defaultOTLPEndpoint  = ""
	defaultSampleRate    = 1.0
)

var defaultKafkaBrokers = []string{"localhost:9092"}

func Parse() (*Config, error) {
	cfg := &Config{}

	pflag.StringVarP(&cfg.RunAddress, "address", "a", defaultRunAddress, "адрес и порт HTTP-сервера")
	pflag.StringVarP(&cfg.DatabaseURI, "database", "d", defaultDatabaseURI, "строка подключения к PostgreSQL")
	pflag.StringVarP(&cfg.LogLevel, "loglevel", "l", defaultLogLevel, "уровень логирования")
	pflag.StringVar(&cfg.StaticDir, "static", defaultStaticDir, "каталог со статикой веб-интерфейса")
	pflag.Int64Var(&cfg.MaxUploadSize, "max-upload-size", defaultMaxUploadSize, "максимальный размер файла в байтах")

	pflag.StringVar(&cfg.S3Endpoint, "s3-endpoint", defaultS3Endpoint, "адрес S3-совместимого хранилища")
	pflag.StringVar(&cfg.S3AccessKey, "s3-access-key", defaultS3AccessKey, "ключ доступа к хранилищу")
	pflag.StringVar(&cfg.S3SecretKey, "s3-secret-key", defaultS3SecretKey, "секретный ключ хранилища")
	pflag.StringVar(&cfg.S3Bucket, "s3-bucket", defaultS3Bucket, "бакет для файлов аватарок")
	pflag.BoolVar(&cfg.S3UseSSL, "s3-use-ssl", false, "использовать HTTPS при обращении к хранилищу")

	pflag.StringSliceVar(&cfg.KafkaBrokers, "kafka-brokers", defaultKafkaBrokers, "адреса брокеров Kafka")
	pflag.StringVar(&cfg.KafkaTopic, "kafka-topic", defaultKafkaTopic, "топик событий об аватарках")
	pflag.StringVar(&cfg.KafkaGroupID, "kafka-group", defaultKafkaGroupID, "consumer-группа воркера")

	pflag.StringVar(&cfg.OTLPEndpoint, "otlp-endpoint", defaultOTLPEndpoint,
		"адрес OTLP-коллектора; пусто — телеметрия выключена")
	pflag.Float64Var(&cfg.TraceSampleRate, "trace-sample-rate", defaultSampleRate, "доля сэмплируемых трейсов")

	pflag.Parse()

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config.Parse: %w", err)
	}

	return cfg, nil
}
