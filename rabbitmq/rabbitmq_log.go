package rabbitmq

import "github.com/RichardKnop/machinery/v2/log"

func setLog() {
	log.SetInfo(&NoLog{})
	log.SetDebug(&NoLog{})
}

// NoLog implements log.Logger interface
type NoLog struct {
}

// Print ...
func (w *NoLog) Print(v ...interface{}) {
}

// Printf ...
func (w *NoLog) Printf(format string, v ...interface{}) {
}

// Println ...
func (w *NoLog) Println(v ...interface{}) {
}

// Fatal ...
func (w *NoLog) Fatal(v ...interface{}) {
}

// Fatalf ...
func (w *NoLog) Fatalf(format string, v ...interface{}) {
}

// Fatalln ...
func (w *NoLog) Fatalln(v ...interface{}) {
}

// Panic ...
func (w *NoLog) Panic(v ...interface{}) {
}

// Panicf ...
func (w *NoLog) Panicf(format string, v ...interface{}) {
}

// Panicln ...
func (w *NoLog) Panicln(v ...interface{}) {
}
