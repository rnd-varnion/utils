package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/rnd-varnion/utils/elastic"
	"github.com/rnd-varnion/utils/influx"
	mongoPing "github.com/rnd-varnion/utils/mongo"
	"github.com/rnd-varnion/utils/mysql"
	"github.com/rnd-varnion/utils/postgres"
	redisPing "github.com/rnd-varnion/utils/redis"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	CONNECTION_TYPE string    = "CONNECTION_TYPE"
	CONNECTION_NAME [6]string = [6]string{
		"ELASTIC",
		"INFLUX",
		"MONGO",
		"MYSQL",
		"POSTGRES",
		"REDIS",
	}
)

func AddMiddleware(r *gin.Engine, h ...gin.HandlerFunc) {
	for _, v := range h {
		r.Use(v)
	}
}

type Connection struct {
	CTX    context.Context
	DSN    string
	ES     *elasticsearch.Client
	INFLUX influxdb2.Client
	MONGO  *mongo.Client
	REDIS  *redis.Client
}

func (conn *Connection) GetConnection(ConnectionName string) gin.HandlerFunc {
	if ConnectionName == "" {
		ConnectionName = os.Getenv(CONNECTION_TYPE)
	}

	switch ConnectionName {
	case CONNECTION_NAME[0]:
		return func(c *gin.Context) {
			err := elastic.Ping(conn.CTX, conn.ES)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}
	case CONNECTION_NAME[1]:
		return func(c *gin.Context) {
			err := influx.Ping(conn.CTX, conn.INFLUX)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}
	case CONNECTION_NAME[2]:
		return func(c *gin.Context) {
			err := mongoPing.Ping(conn.CTX, conn.MONGO)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}

	case CONNECTION_NAME[3]:
		return func(c *gin.Context) {
			err := mysql.Ping(conn.DSN)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}

	case CONNECTION_NAME[4]:
		return func(c *gin.Context) {
			err := postgres.Ping(conn.DSN)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}

	case CONNECTION_NAME[5]:
		return func(c *gin.Context) {
			err := redisPing.Ping(conn.CTX, conn.REDIS)
			if err != nil {
				c.JSON(
					http.StatusBadGateway,
					gin.H{
						"error": err.Error(),
					},
				)
				c.Abort()
			}

			c.Next()
		}

	}

	return func(c *gin.Context) {
		c.JSON(
			http.StatusBadGateway,
			gin.H{
				"error": fmt.Errorf("connection not found"),
			},
		)

		c.Abort()
	}
}
