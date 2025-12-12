package kafka

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/IBM/sarama"
	"github.com/tsumida/lunaship/infra"
	"go.uber.org/zap"

	iutils "github.com/tsumida/tex/pkg/utils"
)

// 参考: https://github.com/IBM/sarama/blob/main/examples/consumergroup/main.go
// 考虑了pause, rebalance等场景

type KafkaConsumer struct {
	Brokers       string
	ConsumerGroup string
	Topic         string
}

func (c *KafkaConsumer) Start(
	cctx context.Context,
	cfg *sarama.Config,
	consumer *ConsumerWrapper,
) error {
	keepRunning := true
	l := infra.GlobalLog().With(
		zap.String("brokers", c.Brokers), zap.String("consumer_group", c.ConsumerGroup), zap.String("topic", c.Topic),
	)
	ctx, cancel := context.WithCancel(cctx)
	l.Info("initializing kafka consumer")
	defer l.Info("kafka consumer down")

	client, err := sarama.NewConsumerGroup(strings.Split(c.Brokers, ","), c.ConsumerGroup, cfg)
	if err != nil {
		l.Error("Error creating consumer group client", zap.Error(err))
		cancel()
		return err
	}

	if consumer == nil {
		cancel()
		return errors.New("kafka consumer handler is nil")
	}

	// ensure we have a fresh ready channel before starting the loop
	consumer.ready = make(chan bool)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// `Consume` should be called inside an infinite loop, when a
			// server-side rebalance happens, the consumer session will need to be
			// recreated to get the new claims
			if err := client.Consume(ctx, []string{c.Topic}, consumer); err != nil {
				if errors.Is(err, sarama.ErrClosedConsumerGroup) {
					return
				}
				log.Panicf("Error from consumer: %v", err)
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				return
			}
			consumer.ready = make(chan bool)
		}
	}()

	<-consumer.ready // Await till the consumer has been set up
	log.Println("Sarama consumer up and running!...")

	// note: SIGUSR1, SIGUSR2用来开启pprof
	// sigusr1 := make(chan os.Signal, 1)
	// signal.Notify(sigusr1, syscall.SIGUSR1)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	for keepRunning {
		select {
		case <-ctx.Done():
			l.Info("terminating: context cancelled")
			keepRunning = false
		case <-sigterm:
			l.Info("terminating: via signal")
			keepRunning = false
		}
	}
	cancel()
	wg.Wait()
	return client.Close()
}

type MsgHandlerFunc func(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error

type ConsumerWrapper struct {
	iutils.ModuleAPI

	name    string
	ready   chan bool
	handler MsgHandlerFunc
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *ConsumerWrapper) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *ConsumerWrapper) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
// Once the Messages() channel is closed, the Handler must finish its processing
// loop and exit.
func (consumer *ConsumerWrapper) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	l := infra.GlobalLog()
	// todo: use sync.Pool
	logTags := make([]zap.Field, 0, 12)

	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/IBM/sarama/blob/main/consumer_group.go#L27-L29
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				l.Info("message channel was closed")
				return nil
			}
			logTags = append(
				logTags,
				zap.String("consumer", consumer.name),
				zap.String("topic", message.Topic),
				zap.Int32("partition", message.Partition),
				zap.Int64("offset", message.Offset),
			)
			l.Info("consume msg", logTags...)
			if consumer.handler != nil {
				if err := consumer.handler(session, message); err != nil {
					logTags = append(logTags, zap.Error(err))
					l.Error("message handler error", zap.Error(err))
				}
			}
			logTags = logTags[:0]
		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalance. see:
		// https://github.com/IBM/sarama/issues/1192
		case <-session.Context().Done():
			return nil
		}
	}
}

func NewConsumerWrapper(name string, f MsgHandlerFunc) *ConsumerWrapper {
	return &ConsumerWrapper{
		name:    name,
		ready:   make(chan bool),
		handler: f,
	}
}

func DefaultConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	// ack = -1, insync replicas=all
	// autocommit = false
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 3
	cfg.Producer.Return.Successes = true
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Offsets.AutoCommit.Enable = false
	return cfg
}
