package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rnd-varnion/utils/logger"
)

var (
	MINIO_ENDPOINT   = "MINIO_ENDPOINT"
	MINIO_ACCESS_KEY = "MINIO_ACCESS_KEY"
	MINIO_SECRET_KEY = "MINIO_SECRET_KEY"
	MINIO_BUCKET     = "MINIO_BUCKET"
)

type Storage struct {
	MinioClient *minio.Client
}

type UploadPayload struct {
	Folder      string
	File        io.Reader
	Filename    string
	ContentType string
	Size        int64
}

func NewStorage() (*Storage, error) {
	caCertPath := os.Getenv("MINIO_PATH_CERT")
	endpoint := os.Getenv(MINIO_ENDPOINT)
	accessKeyID := os.Getenv(MINIO_ACCESS_KEY)
	secretAccessKey := os.Getenv(MINIO_SECRET_KEY)

	if endpoint == "" || accessKeyID == "" || secretAccessKey == "" {
		logger.Log.Fatalf("missing required environment variables")
	}

	var transport *http.Transport

	if caCertPath == "" {
		// No CA → skip verification
		logger.Log.Info("No MINIO_PATH_CERT set, using TLS but skipping verification")
		tlsConfig := &tls.Config{InsecureSkipVerify: true} // dev only!
		transport = http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = tlsConfig
	} else {
		// Load CA cert
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			logger.Log.Fatalf("failed to read CA cert: %v", err)
		}

		// Append CA to system pool
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			logger.Log.Fatalf("failed to load system cert pool: %v", err)
		}
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			logger.Log.Fatalf("failed to append CA cert")
		}

		tlsConfig := &tls.Config{RootCAs: caCertPool}
		transport = http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = tlsConfig

		logger.Log.Info("Using TLS with custom CA")
	}

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure:    true,
		Transport: transport,
	})
	if err != nil {
		logger.Log.Fatalln("Failed to initialize storage minio, err: ", err)
		return nil, err
	}

	storage := &Storage{
		MinioClient: minioClient,
	}

	return storage, nil
}

func (s *Storage) UploadFile(ctx context.Context, payload *UploadPayload) error {
	filePath := fmt.Sprintf("%s/%s", payload.Folder, payload.Filename)

	_, err := s.MinioClient.PutObject(
		ctx,
		os.Getenv(MINIO_BUCKET),
		filePath,
		payload.File,
		payload.Size,
		minio.PutObjectOptions{
			ContentType: payload.ContentType,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) DeleteFile(ctx context.Context, folder, filename string) error {
	filePath := fmt.Sprintf("%s/%s", folder, filename)

	err := s.MinioClient.RemoveObject(
		ctx,
		os.Getenv(MINIO_BUCKET),
		filePath,
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) IsFileExists(ctx context.Context, folder, filename string) bool {
	filePath := fmt.Sprintf("%s/%s", folder, filename)

	_, err := s.MinioClient.StatObject(ctx, os.Getenv(MINIO_BUCKET), filePath, minio.StatObjectOptions{})
	if err != nil {
		respErr, ok := err.(minio.ErrorResponse)
		if ok && respErr.Code == "NoSuchKey" {
			return false
		}
		return false
	}
	return true
}

func (s *Storage) CopyFile(ctx context.Context, folder, filename, newFilename string) error {
	bucket := os.Getenv(MINIO_BUCKET)

	srcPath := fmt.Sprintf("%s/%s", folder, filename)
	destPath := fmt.Sprintf("%s/%s", folder, newFilename)

	src := minio.CopySrcOptions{
		Bucket: bucket,
		Object: srcPath,
	}

	dst := minio.CopyDestOptions{
		Bucket: bucket,
		Object: destPath,
	}

	_, err := s.MinioClient.CopyObject(ctx, dst, src)
	return err
}
