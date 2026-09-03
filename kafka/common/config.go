package common

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rnd-varnion/utils/logger"
)

// Environment variable names
const (
	KAFKA_BROKERS        = "KAFKA_BROKERS"
	KAFKA_CLIENT_ID      = "KAFKA_CLIENT_ID"
	KAFKA_USERNAME       = "KAFKA_USERNAME"
	KAFKA_PASSWORD       = "KAFKA_PASSWORD"
	KAFKA_CA_CERT        = "KAFKA_CA_CERT"
	KAFKA_SASL_MECHANISM = "KAFKA_SASL_MECHANISM"
)

// Config holds Kafka configuration
type Config struct {
	Brokers        []string
	ClientID       string
	Username       string
	Password       string
	CACertPath     string
	SASLMechanism  string
	RequestTimeout time.Duration
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	_ = godotenv.Load()

	brokers := os.Getenv(KAFKA_BROKERS)
	if brokers == "" {
		brokers = "localhost:9092" // fallback
	}

	clientID := os.Getenv(KAFKA_CLIENT_ID)
	if clientID == "" {
		clientID = "varnion-kafka-client"
	}

	username := os.Getenv(KAFKA_USERNAME)
	password := os.Getenv(KAFKA_PASSWORD)
	caCert := os.Getenv(KAFKA_CA_CERT)
	saslMechanism := os.Getenv(KAFKA_SASL_MECHANISM)

	if saslMechanism == "" {
		saslMechanism = "sha256" // default
	}

	config := &Config{
		Brokers:        []string{brokers},
		ClientID:      clientID,
		Username:      username,
		Password:      password,
		CACertPath:    caCert,
		SASLMechanism: saslMechanism,
		RequestTimeout: 10 * time.Second, // default timeout
	}

	logger.Log.Infof("[INFO] Loaded Kafka config - Brokers: %v, ClientID: %s\n", config.Brokers, config.ClientID)

	return config
}
