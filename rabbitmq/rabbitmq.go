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

type ConsumeAckConfig struct {
	Vhost         string
	Queue         string
	PrefetchCount int // Number of unacknowledged messages allowed
}

type PublishConfig struct {
	Vhost   string
	Queue   string
	Message string
}

// Message represents a message with manual acknowledgment capability
type Message struct {
	Body        string
	DeliveryTag uint64
	ack         func() error
	nack        func(requeue bool) error
}

// Ack acknowledges the message
func (m *Message) Ack() error {
	if m.ack == nil {
		return fmt.Errorf("ack function not available")
	}
	return m.ack()
}

// Nack negatively acknowledges the message
func (m *Message) Nack(requeue bool) error {
	if m.nack == nil {
		return fmt.Errorf("nack function not available")
	}
	return m.nack(requeue)
}

// Reject rejects the message (same as Nack with requeue=false)
func (m *Message) Reject() error {
	return m.Nack(false)
}

// Requeue rejects and requeues the message
func (m *Message) Requeue() error {
	return m.Nack(true)
}

type Engine struct {
	cfg     NewConfig
	servers map[string]*machinery.Server
	workers map[string]*machinery.Worker
	mu      sync.Mutex
}

func New(cfg NewConfig) *Engine {
	return &Engine{
		cfg:     cfg,
		servers: make(map[string]*machinery.Server),
		workers: make(map[string]*machinery.Worker),
	}
}

func (e *Engine) getServer(vhost, queue string, prefetchCount int) (*machinery.Server, error) {
	setLog()

	key := fmt.Sprintf("%s:%s:%d", vhost, queue, prefetchCount)

	e.mu.Lock()
	defer e.mu.Unlock()

	if srv, ok := e.servers[key]; ok {
		return srv, nil
	}

	rabbitURL := fmt.Sprintf("amqp://%s:%s@%s/%s",
		e.cfg.Username, e.cfg.Password, e.cfg.Host, vhost)

	if prefetchCount <= 0 {
		prefetchCount = 1
	}

	cnf := &machineryConfig.Config{
		Broker:        rabbitURL,
		DefaultQueue:  queue,
		ResultBackend: rabbitURL,
		AMQP: &machineryConfig.AMQPConfig{
			Exchange:      queue,
			ExchangeType:  "direct",
			BindingKey:    queue,
			PrefetchCount: prefetchCount,
		},
	}

	broker := amqpbroker.New(cnf)
	backend := amqpbackend.New(cnf)
	lock := eagerlock.New()

	srv := machinery.NewServer(cnf, broker, backend, lock)
	e.servers[key] = srv
	return srv, nil
}

// Consume returns a channel that receives messages (auto-ack)
func (e *Engine) Consume(cfg ConsumeConfig) (<-chan string, error) {
	setLog()

	srv, err := e.getServer(cfg.Vhost, cfg.Queue, 1)
	if err != nil {
		return nil, err
	}

	out := make(chan string, 1000)

	err = srv.RegisterTask(cfg.Queue, func(msg string) error {
		out <- msg
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register task: %w", err)
	}

	workerName := fmt.Sprintf("worker-%s-%s", cfg.Vhost, cfg.Queue)
	worker := srv.NewWorker(workerName, 0)

	go func() {
		if err := worker.Launch(); err != nil {
			fmt.Printf("worker error on queue %s: %v\n", cfg.Queue, err)
			close(out)
		}
	}()

	e.mu.Lock()
	e.workers[fmt.Sprintf("%s:%s", cfg.Vhost, cfg.Queue)] = worker
	e.mu.Unlock()

	return out, nil
}

// ConsumeAck returns a channel that receives messages requiring manual acknowledgment
func (e *Engine) ConsumeAck(cfg ConsumeAckConfig) (<-chan *Message, error) {
	setLog()

	if cfg.PrefetchCount <= 0 {
		cfg.PrefetchCount = 1
	}

	srv, err := e.getServer(cfg.Vhost, cfg.Queue, cfg.PrefetchCount)
	if err != nil {
		return nil, err
	}

	out := make(chan *Message, cfg.PrefetchCount)
	ackMap := &sync.Map{} // Store acknowledgment functions by delivery tag

	var deliveryCounter uint64
	var counterMu sync.Mutex

	err = srv.RegisterTask(cfg.Queue, func(msg string) error {
		counterMu.Lock()
		deliveryCounter++
		currentTag := deliveryCounter
		counterMu.Unlock()

		ackChan := make(chan error, 1)
		nackChan := make(chan struct {
			requeue bool
			err     error
		}, 1)

		message := &Message{
			Body:        msg,
			DeliveryTag: currentTag,
			ack: func() error {
				ackChan <- nil
				return <-ackChan
			},
			nack: func(requeue bool) error {
				nackChan <- struct {
					requeue bool
					err     error
				}{requeue: requeue}
				result := <-nackChan
				return result.err
			},
		}

		ackMap.Store(currentTag, struct {
			ackChan  chan error
			nackChan chan struct {
				requeue bool
				err     error
			}
		}{ackChan: ackChan, nackChan: nackChan})

		out <- message

		// Wait for ack or nack
		select {
		case <-ackChan:
			ackMap.Delete(currentTag)
			ackChan <- nil
			return nil
		case nackInfo := <-nackChan:
			ackMap.Delete(currentTag)
			if nackInfo.requeue {
				nackChan <- struct {
					requeue bool
					err     error
				}{err: nil}
				return fmt.Errorf("message requeued")
			}
			nackChan <- struct {
				requeue bool
				err     error
			}{err: nil}
			return fmt.Errorf("message rejected")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register task: %w", err)
	}

	workerName := fmt.Sprintf("worker-manual-%s-%s", cfg.Vhost, cfg.Queue)
	worker := srv.NewWorker(workerName, 0)

	go func() {
		if err := worker.Launch(); err != nil {
			fmt.Printf("worker error on queue %s: %v\n", cfg.Queue, err)
			close(out)
		}
	}()

	e.mu.Lock()
	e.workers[fmt.Sprintf("manual:%s:%s", cfg.Vhost, cfg.Queue)] = worker
	e.mu.Unlock()

	return out, nil
}

func (e *Engine) Publish(cfg PublishConfig) error {
	srv, err := e.getServer(cfg.Vhost, cfg.Queue, 1)
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
	srv, err := e.getServer(cfg.Vhost, cfg.Queue, 1)
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

// Shutdown gracefully shuts down all workers
func (e *Engine) Shutdown() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key, worker := range e.workers {
		worker.Quit()
		fmt.Printf("Shut down worker: %s\n", key)
	}

	return nil
}

// CloseServer removes a server from the cache
func (e *Engine) CloseServer(vhost, queue string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := fmt.Sprintf("%s:%s", vhost, queue)
	delete(e.servers, key)
	delete(e.workers, key)
}
