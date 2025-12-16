package pkg

import (
	"context"
	"os"
	"time"

	connect "connectrpc.com/connect"
	v9 "github.com/redis/go-redis/v9"
	"github.com/tsumida/lunaship/infra"
	"github.com/tsumida/lunaship/interceptor"
	"github.com/tsumida/lunaship/kafka"
	"github.com/tsumida/lunaship/log"
	"github.com/tsumida/lunaship/redis"
	service "github.com/tsumida/lunaship/service"
	"github.com/tsumida/lunaship/utils"
	svc "github.com/tsumida/tex/gen/api/apiconnect"
	redisstate "github.com/tsumida/tex/pkg/state/redis_state"
	"go.uber.org/zap"
)

// func initDB(env string) func() error {
// 	switch env {
// 	case "dev", "test":
// 		return func() error {
// 			return infra.InitMySQL(
// 				mysql.Config{
// 					DSN: fmt.Sprintf(
// 						"%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true",
// 						utils.StrOrDefault(os.Getenv("MYSQL_USER"), ""),
// 						utils.StrOrDefault(os.Getenv("MYSQL_PWD"), ""),
// 						utils.StrOrDefault(os.Getenv("MYSQL_ADDR"), "localhost:3306"),
// 						utils.StrOrDefault(os.Getenv("MYSQL_DB"), "tex"),
// 					),
// 				},
// 				gorm.Config{},
// 				func(_ *gorm.DB) error { return nil },
// 			)
// 		}
// 	default:
// 		panic("invalid env")
// 	}
// }

func RunApp(
	ctx context.Context,
) {
	path, handler := svc.NewTexServiceHandler(
		NewTexService(),
		connect.WithRecover(infra.RecoverFn),
		connect.WithInterceptors(
			interceptor.NewReqRespLogger(),
		),
	)

	svc := &service.Service{
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
			return redis.InitRedis(ctx, &v9.UniversalOptions{
				Addrs: []string{utils.StrOrDefault(os.Getenv("REDIS_ADDR"), "localhost:6379")},
			},
				2*time.Second, 2,
			)
		},
		func() error {
			client := redis.GlobalRedis()
			ledgerUpdater := redisstate.NewLedgerHandler(client)
			orderUpdater := redisstate.NewOrderHandler(client)
			kBarUpdater := redisstate.NewKBarUpdator(client)

			// if err := iutils.AnyError(
			// 	ledgerUpdater.PrepareLuaScript(ctx),
			// 	orderUpdater.PrepareLuaScript(ctx),
			// 	kBarUpdater.PrepareLuaScript(ctx),
			// ); err != nil {
			// 	panic(err)
			// }

			go utils.GoWithAction(func() {
				cg := "tex_match_result"
				topic := "match_result_BTCUSDT"
				consumer := &kafka.KafkaConsumer{
					Brokers:       utils.StrOrDefault(os.Getenv("KAFKA_BROKERS"), "kafka-dev:9092"),
					Topic:         topic,
					ConsumerGroup: cg,
				}
				cfg := kafka.DefaultConfig()

				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("match_result_consumer", HandleMatchResultEvent(kBarUpdater))); err != nil {
					log.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					log.GlobalLog().Error("ledger event consumer panicked", zap.Any("recover", r))
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
				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("order_event_consumer", HandleOrderEvent(orderUpdater))); err != nil {
					log.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					log.GlobalLog().Error("order event consumer panicked", zap.Any("recover", r))
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
				if err := consumer.Start(ctx, cfg, kafka.NewConsumerWrapper("ledger_event_consumer", HandleLedgerEvent(ledgerUpdater))); err != nil {
					log.GlobalLog().Error("failed to start kafka consumer", zap.String("topic", topic), zap.String("consumer_group", cg), zap.Error(err))
				}
			},
				func(r any) {
					log.GlobalLog().Error("ledger event consumer panicked", zap.Any("recover", r))
				},
			)
			return nil
		},
	)
}
