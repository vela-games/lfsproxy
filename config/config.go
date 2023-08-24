package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DebugMode                bool          `split_words:"true" default:"false"`
	UpstreamBaseURL          string        `split_words:"true" required:"true"`
	CacheEviction            time.Duration `split_words:"true" default:"23h"`
	S3Bucket                 string        `split_words:"true" required:"true"`
	S3UseAccelerate          bool          `split_words:"true" default:"false"`
	S3PresignEnabled         bool          `split_words:"true" default:"true"`
	S3PresignExpiration      time.Duration `split_words:"true" default:"24h"`
	EnablePrometheusExporter bool          `split_words:"true" default:"false"`
}

func GetConfig() (*Config, error) {
	var proxyConfiguration Config

	err := envconfig.Process("app", &proxyConfiguration)
	if err != nil {
		return nil, err
	}

	return &proxyConfiguration, nil
}
