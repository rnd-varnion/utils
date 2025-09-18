package http_client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rnd-varnion/utils/logger"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

func init() {
	isUseLogstash := os.Getenv(logger.LOG_API_USE_LOGSTASH) == "true"
	Log = logger.InitCustomLogger("api_request", "info", isUseLogstash)
}

type RequestConfig struct {
	Method     string
	URL        string
	Headers    map[string]string
	Body       []byte
	TimeoutSec int
	Transport  http.RoundTripper
}

type ResponseResult struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

func DoRequestWithLog(cfg RequestConfig) (*ResponseResult, error) {
	timeout := 5 * time.Second
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: cfg.Transport,
	}

	start := time.Now()

	req, err := http.NewRequest(cfg.Method, cfg.URL, bytes.NewBuffer(cfg.Body))
	if err != nil {
		return nil, fmt.Errorf("failed create request: %w", err)
	}

	for key, val := range cfg.Headers {
		req.Header.Set(key, val)
	}

	resp, err := client.Do(req)

	duration := time.Since(start)
	entry := Log.WithFields(logrus.Fields{
		"method": req.Method,
		"url":    req.URL.String(),
		"request": logrus.Fields{
			"headers": req.Header,
			"body":    string(cfg.Body),
		},
		"response_time": duration.Milliseconds(),
	})

	if err != nil {
		entry.WithError(err).Error("API call failed")
		return nil, fmt.Errorf("failed http request: %w", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		entry.WithError(err).Error("Failed to read response body")
		return nil, fmt.Errorf("failed read body: %w", err)
	}

	entry = entry.WithFields(logrus.Fields{
		"response": logrus.Fields{
			// "headers": resp.Header,
			"body": string(respBody),
		},
		"status_code": resp.StatusCode,
	})
	entry.Info("API call success")

	return &ResponseResult{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

func DoRequest(cfg RequestConfig) (*ResponseResult, error) {
	timeout := 5 * time.Second
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: cfg.Transport,
	}

	req, err := http.NewRequest(cfg.Method, cfg.URL, bytes.NewBuffer(cfg.Body))
	if err != nil {
		return nil, fmt.Errorf("failed create request: %w", err)
	}

	for key, val := range cfg.Headers {
		req.Header.Set(key, val)
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("failed http request: %w", err)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed read body: %w", err)
	}

	return &ResponseResult{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}
