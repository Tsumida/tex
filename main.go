package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/go-redis/redis"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/lunaship/infra/utils"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	// pb "github.com/tsumida/tex/pb/connect/api/api"
	svc "github.com/tsumida/tex/gen/api/apiconnect"
	"github.com/tsumida/tex/internal"
	"github.com/tsumida/tex/internal/kafka"
	redisstate "github.com/tsumida/tex/internal/state/redis_state"
	iutils "github.com/tsumida/tex/internal/utils"
)

func initDB(env string) func() error {
	switch env {
	case "dev", "test":
		return func() error {
			return infra.InitMySQL(
				mysql.Config{
					DSN: fmt.Sprintf(
						"%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true",
						utils.StrOrDefault(os.Getenv("MYSQL_USER"), ""),
						utils.StrOrDefault(os.Getenv("MYSQL_PWD"), ""),
						utils.StrOrDefault(os.Getenv("MYSQL_ADDR"), "localhost:3306"),
						utils.StrOrDefault(os.Getenv("MYSQL_DB"), "tex"),
					),
				},
				gorm.Config{},
				func(_ *gorm.DB) error { return nil },
			)
		}
	default:
		panic("invalid env")
	}
}

func RunTex(ctx context.Context) {
	path, handler := svc.NewTexServiceHandler(
		internal.NewTexService(),
		connect.WithRecover(infra.RecoverFn),
		connect.WithInterceptors(
			infra.NewReqRespLogger(),
		),
	)

	svc := &infra.Service{
		Path:           path,
		Handler:        handler,
		BindingAddress: utils.StrOrDefault(os.Getenv("SERVER_BIND_ADDR"), ":8180"),
	}

	svc.RunAfterInit(
		ctx,
		10*time.Second,
		func() error {
			go infra.InitMetric(infra.PROMETHEUS_LISTEN_ADDR)
			return nil
		},
		func() error {
			return infra.InitGopprof(infra.DEFAULT_PPROF_ADDR)
		},
		func() error {
			return infra.InitRedis(ctx, &redis.UniversalOptions{
				Addrs: []string{utils.StrOrDefault(os.Getenv("REDIS_ADDR"), "localhost:6379")},
			})
		},
		func() error {
			client := infra.GlobalRedis()
			ledgerUpdater := redisstate.NewLedgerHandler(client)
			orderUpdater := redisstate.NewOrderHandler(client)
			kBarUpdater := redisstate.NewKBarUpdator(client)

			if err := iutils.AnyError(
				ledgerUpdater.PrepareLuaScript(ctx),
				orderUpdater.PrepareLuaScript(ctx),
				kBarUpdater.PrepareLuaScript(ctx),
			); err != nil {
				panic(err)
			}

			go utils.GoWithAction(func() {
				cg := "tex_match_result_cg"
				topic := "match_result_BTCUSDT"
				consumer := &kafka.KafkaConsumer{
					Brokers:       utils.StrOrDefault(os.Getenv("KAFKA_BROKERS"), "kafka-dev:9092"),
					Topic:         topic,
					ConsumerGroup: cg,
				}
				cfg := kafka.DefaultConfig()

				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("match_result_consumer", internal.HandleMatchResultEvent(kBarUpdater))); err != nil {
					infra.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					infra.GlobalLog().Error("ledger event consumer panicked", zap.Any("recover", r))
				},
			)

			go utils.GoWithAction(func() {
				cg := "tex_order_events"
				topic := "order_events"
				consumer := &kafka.KafkaConsumer{
					Brokers:       utils.StrOrDefault(os.Getenv("KAFKA_BROKERS"), "kafka-dev:9092"),
					Topic:         topic,
					ConsumerGroup: cg,
				}
				cfg := kafka.DefaultConfig()
				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("order_event_consumer", internal.HandleOrderEvent(orderUpdater))); err != nil {
					infra.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					infra.GlobalLog().Error("order event consumer panicked", zap.Any("recover", r))
				},
			)

			go utils.GoWithAction(func() {
				cg := "tex_ledger_events"
				topic := "ledger_events"
				consumer := &kafka.KafkaConsumer{
					Brokers:       utils.StrOrDefault(os.Getenv("KAFKA_BROKERS"), "kafka-dev:9092"),
					Topic:         topic,
					ConsumerGroup: cg,
				}
				cfg := kafka.DefaultConfig()
				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("ledger_event_consumer", internal.HandleLedgerEvent(ledgerUpdater))); err != nil {
					infra.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					infra.GlobalLog().Error("ledger event consumer panicked", zap.Any("recover", r))
				},
			)
			return nil
		},
	)
}

func main() {
	var (
		ctx = context.Background()
	)
	RunTex(ctx)
}
