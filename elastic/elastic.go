package elastic

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/rnd-varnion/utils/logger"
)

var (
	ES_HOST = "ES_HOST"
	ES_USER = "ES_USER"
	ES_PASS = "ES_PASS"
)

// T is a generic type for the response source.
// Example: var usersResp Response[User]
type Response[T any] struct {
	Took    int  `json:"took"`
	Timeout bool `json:"timed_out"`
	Hits    struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source T `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func InitDB() *elasticsearch.Client {
	config := elasticsearch.Config{
		Addresses: []string{
			os.Getenv(ES_HOST),
		},
		Username: os.Getenv(ES_USER),
		Password: os.Getenv(ES_PASS),
	}

	es, err := elasticsearch.NewClient(config)
	if err != nil {
		logger.Log.Fatalf("error creating elastic search : %v", err)
		return nil
	}

	return es
}

func Ping(ctx context.Context, db *elasticsearch.Client) error {
	ctxx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	res, err := db.Cluster.Health(
		db.Cluster.Health.WithContext(ctxx),
		db.Cluster.Health.WithPretty(),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch unhealthy: %s", res.Status())
	}

	return nil
}
