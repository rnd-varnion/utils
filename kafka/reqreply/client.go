package reqreply

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Client represents a Kafka client for request-reply operations
type Client struct {
	producer *kgo.Client
	consumer *kgo.Client
	config   *common.Config
}

// NewClient creates a new Kafka client for request-reply operations
func NewClient(config *common.Config) (*Client, error) {
	if config == nil {
		config = common.LoadConfigFromEnv()
	}

	logger.Log.Infof("[INFO] Creating Kafka client for brokers: %v\n", config.Brokers)

	// Create producer opts
	producerOpts := []kgo.Opt{
		kgo.SeedBrokers(config.Brokers...),
		kgo.ClientID(config.ClientID + "-producer"),
		kgo.ProduceRequestTimeout(10 * time.Second),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	}

	// Add SASL authentication if configured
	if config.Username != "" && config.Password != "" {
		// For now, skip SASL configuration - this requires proper implementation
		// TODO: Implement proper SASL authentication with franz-go
		logger.Log.Warn("[WARN] SASL authentication requested but not yet implemented")
	}

	// Add TLS if configured
	if config.CACertPath != "" {
		caCert, err := os.ReadFile(config.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS12,
		}

		producerOpts = append(producerOpts, kgo.DialTLSConfig(tlsConfig))
	}

	// Create consumer opts
	consumerOpts := []kgo.Opt{
		kgo.SeedBrokers(config.Brokers...),
		kgo.ClientID(config.ClientID + "-consumer"),
		kgo.ProduceRequestTimeout(10 * time.Second),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	}

	if config.CACertPath != "" {
		caCert, err := os.ReadFile(config.CACertPath)
		if err == nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)

			tlsConfig := &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: tls.VersionTLS12,
			}
			consumerOpts = append(consumerOpts, kgo.DialTLSConfig(tlsConfig))
		}
	}

	// Create producer
	producer, err := kgo.NewClient(producerOpts...)
	if err != nil {
		logger.Log.Errorf("[ERROR] Failed to create producer: %v\n", err)
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	// Create consumer
	consumer, err := kgo.NewClient(consumerOpts...)
	if err != nil {
		logger.Log.Errorf("[ERROR] Failed to create consumer: %v\n", err)
		producer.Close()
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	client := &Client{
		producer: producer,
		consumer: consumer,
		config:   config,
	}

	logger.Log.Infof("[INFO] Kafka client created successfully\n")
	return client, nil
}

// GetProducer returns the producer client
func (c *Client) GetProducer() *kgo.Client {
	return c.producer
}

// GetConsumer returns the consumer client
func (c *Client) GetConsumer() *kgo.Client {
	return c.consumer
}

// Ping checks if the Kafka connection is healthy
func (c *Client) Ping(ctx context.Context) error {
	// Try to fetch metadata to check connection
	// Use the broker's internal metadata fetch to test connection
	if err := c.producer.Ping(ctx); err != nil {
		return fmt.Errorf("kafka connection unhealthy: %w", err)
	}

	logger.Log.Info("[INFO] Kafka connection is healthy")
	return nil
}

// Close closes both producer and consumer connections
func (c *Client) Close() error {
	logger.Log.Info("[INFO] Closing Kafka client")

	if c.producer != nil {
		c.producer.Close()
	}

	if c.consumer != nil {
		c.consumer.Close()
	}

	logger.Log.Info("[INFO] Kafka client closed successfully")
	return nil
}
