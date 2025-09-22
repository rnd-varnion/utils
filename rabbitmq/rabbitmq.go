package rabbitmq

import (
	"fmt"
	"sync"
	"time"

	"github.com/RichardKnop/machinery/v2"
	amqpbackend "github.com/RichardKnop/machinery/v2/backends/amqp"
	amqpbroker "github.com/RichardKnop/machinery/v2/brokers/amqp"
	machineryConfig "github.com/RichardKnop/machinery/v2/config"
	eagerlock "github.com/RichardKnop/machinery/v2/locks/eager"
	"github.com/RichardKnop/machinery/v2/tasks"
)

type NewConfig struct {
	Username string
	Password string
	Host     string
}

type ConsumeConfig struct {
	Vhost string
	Queue string
}

type PublishConfig struct {
	Vhost   string
	Queue   string
	Message string
}

type Engine struct {
	cfg     NewConfig
	servers map[string]*machinery.Server
	mu      sync.Mutex
}

func New(cfg NewConfig) *Engine {
	return &Engine{
		cfg:     cfg,
		servers: make(map[string]*machinery.Server),
	}
}

func (e *Engine) getServer(vhost, queue string) (*machinery.Server, error) {
	setLog()

	key := fmt.Sprintf("%s:%s", vhost, queue)

	e.mu.Lock()
	defer e.mu.Unlock()

	if srv, ok := e.servers[key]; ok {
		return srv, nil
	}

	rabbitURL := fmt.Sprintf("amqp://%s:%s@%s/%s",
		e.cfg.Username, e.cfg.Password, e.cfg.Host, vhost)

	cnf := &machineryConfig.Config{
		Broker:        rabbitURL,
		DefaultQueue:  queue,
		ResultBackend: rabbitURL,
		AMQP: &machineryConfig.AMQPConfig{
			Exchange:      queue,
			ExchangeType:  "direct",
			BindingKey:    queue,
			PrefetchCount: 1,
		},
	}

	broker := amqpbroker.New(cnf)
	backend := amqpbackend.New(cnf)
	lock := eagerlock.New()

	srv := machinery.NewServer(cnf, broker, backend, lock)
	e.servers[key] = srv
	return srv, nil
}

func (e *Engine) Consume(cfg ConsumeConfig) (<-chan string, error) {
	setLog()

	srv, err := e.getServer(cfg.Vhost, cfg.Queue)
	if err != nil {
		return nil, err
	}

	out := make(chan string)

	err = srv.RegisterTask(cfg.Queue, func(msg string) error {
		go func(m string) { out <- m }(msg)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register task: %w", err)
	}

	go func() {
		worker := srv.NewWorker("", 0)
		if err := worker.Launch(); err != nil {
			fmt.Printf("worker error on queue %s: %v\n", cfg.Queue, err)
		}
	}()
	return out, nil
}

func (e *Engine) Publish(cfg PublishConfig) error {
	srv, err := e.getServer(cfg.Vhost, cfg.Queue)
	if err != nil {
		return err
	}

	task := tasks.Signature{
		Name: cfg.Queue,
		Args: []tasks.Arg{
			{Type: "string", Value: cfg.Message},
		},
	}

	_, err = srv.SendTask(&task)
	if err != nil {
		return fmt.Errorf("failed to publish task: %w", err)
	}
	return nil
}

func (e *Engine) PublishWithDelay(cfg PublishConfig, delay time.Duration) error {
	srv, err := e.getServer(cfg.Vhost, cfg.Queue)
	if err != nil {
		return err
	}

	eta := time.Now().Add(delay)

	task := tasks.Signature{
		Name: cfg.Queue,
		Args: []tasks.Arg{
			{Type: "string", Value: cfg.Message},
		},
		ETA: &eta,
	}

	_, err = srv.SendTask(&task)
	if err != nil {
		return fmt.Errorf("failed to publish task: %w", err)
	}
	return nil
}
