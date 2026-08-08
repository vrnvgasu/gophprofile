// Package config содержит конфигурацию приложения.
package config

type Config struct {
	RunAddress    string `env:"RUN_ADDRESS"`
	DatabaseURI   string `env:"DATABASE_URI"`
	LogLevel      string `env:"LOG_LEVEL"`
	StaticDir     string `env:"STATIC_DIR"`
	MaxUploadSize int64  `env:"MAX_UPLOAD_SIZE"`

	S3Endpoint  string `env:"S3_ENDPOINT"`
	S3AccessKey string `env:"S3_ACCESS_KEY"`
	S3SecretKey string `env:"S3_SECRET_KEY"`
	S3Bucket    string `env:"S3_BUCKET"`
	S3UseSSL    bool   `env:"S3_USE_SSL"`

	KafkaBrokers []string `env:"KAFKA_BROKERS" envSeparator:","`
	KafkaTopic   string   `env:"KAFKA_TOPIC"`
	KafkaGroupID string   `env:"KAFKA_GROUP_ID"`
}
