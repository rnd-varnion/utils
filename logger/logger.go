package logger

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	LOG_SERVICE_NAME     = "LOG_SERVICE_NAME"
	LOG_LEVEL            = "LOG_LEVEL"
	LOG_USE_LOGSTASH     = "LOG_USE_LOGSTASH"
	LOG_API_USE_LOGSTASH = "LOG_API_USE_LOGSTASH"
	LOGSTASH_URL         = "LOGSTASH_URL"
)

var conn net.Conn

var Log *logrus.Logger
var ApiRequestLog *logrus.Logger

func init() {
	initLogger()
	initApiRequestLogger()
}

func initLogstash() error {
	if conn != nil {
		return nil
	}

	// Configure the hook to connect to Logstash
	// Retry connection in case Logstash is not ready yet.
	var err error
	for range 3 {
		conn, err = net.Dial("tcp", os.Getenv(LOGSTASH_URL))
		if err == nil {
			break
		}
		fmt.Printf("Failed to connect to Logstash: %v. Retrying...\n", err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("could not connect to Logstash: %w", err)
	}

	return nil
}

func initLogger() {
	Log = logrus.New()

	if os.Getenv(LOG_USE_LOGSTASH) == "true" {
		err := initLogstash()
		if err == nil {
			// Set logger output to the logstash
			Log.SetOutput(conn)
		} else {
			Log.Fatalf("Failed to connect to Logstash: %v", err)
			Log.SetOutput(os.Stdout)
		}
	} else {
		Log.SetOutput(os.Stdout)
	}

	Log.SetFormatter(&logrus.JSONFormatter{})

	// Add the hook here
	Log.AddHook(&ServiceHook{ServiceName: os.Getenv(LOG_SERVICE_NAME)})
	Log.AddHook(NewErrorCallerHook(7))

	level, err := logrus.ParseLevel(os.Getenv(LOG_LEVEL))
	if err != nil {
		level = logrus.InfoLevel // Default to Info level if parsing fails
	}
	Log.SetLevel(level)
}

func initApiRequestLogger() {
	ApiRequestLog = logrus.New()

	if os.Getenv(LOG_API_USE_LOGSTASH) == "true" {
		err := initLogstash()
		if err == nil {
			// Set logger output to the logstash
			ApiRequestLog.SetOutput(conn)
		} else {
			ApiRequestLog.Fatalf("Failed to connect to Logstash: %v", err)
			ApiRequestLog.SetOutput(os.Stdout)
		}
	} else {
		ApiRequestLog.SetOutput(os.Stdout)
	}

	ApiRequestLog.SetFormatter(&logrus.JSONFormatter{})

	// Add the hook here
	ApiRequestLog.AddHook(&ServiceHook{ServiceName: os.Getenv(LOG_SERVICE_NAME)})
	ApiRequestLog.AddHook(&ApiReqHook{})

	ApiRequestLog.SetLevel(logrus.InfoLevel)
}

// ServiceHook adds a "service" field to every log entry.
type ServiceHook struct {
	ServiceName string
}

// Fire is called by logrus for every log entry.
func (hook *ServiceHook) Fire(entry *logrus.Entry) error {
	entry.Data["service"] = hook.ServiceName
	return nil
}

// Levels returns the log levels at which the hook should be fired.
func (hook *ServiceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// ApiReqHook adds a "type" field to every log entry.
type ApiReqHook struct {
}

// Fire is called by logrus for every log entry.
func (hook *ApiReqHook) Fire(entry *logrus.Entry) error {
	entry.Data["type"] = "api_request"
	return nil
}

// Levels returns the log levels at which the hook should be fired.
func (hook *ApiReqHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// ErrorCallerHook is a Logrus hook that adds function info
// only for error-level entries.
type ErrorCallerHook struct {
	// skip is the number of stack frames to skip to find the caller.
	// This might need adjustment if the logging wrappers change.
	skip int
}

// NewErrorCallerHook creates a new hook. A skip of 7 is a good starting point.
func NewErrorCallerHook(skip int) *ErrorCallerHook {
	return &ErrorCallerHook{skip: skip}
}

// Fire adds the caller information to the log entry.
func (hook *ErrorCallerHook) Fire(entry *logrus.Entry) error {
	funcName := ""
	if pc, _, _, ok := runtime.Caller(hook.skip); ok {
		funcName = runtime.FuncForPC(pc).Name()
	}

	entry.Data["func"] = funcName

	return nil
}

// Levels returns the log levels that this hook will be applied to.
func (hook *ErrorCallerHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}
