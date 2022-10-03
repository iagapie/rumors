package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hibiken/asynq"
	"github.com/iagapie/rumors/internal/config"
	"github.com/iagapie/rumors/internal/consts"
	"github.com/iagapie/rumors/internal/events"
	"github.com/iagapie/rumors/internal/http"
	httpHandlers "github.com/iagapie/rumors/internal/http/handlers"
	"github.com/iagapie/rumors/internal/storage"
	"github.com/iagapie/rumors/internal/tasks"
	tasksHandlers "github.com/iagapie/rumors/internal/tasks/handlers"
	"github.com/iagapie/rumors/pkg/emitter"
	"github.com/iagapie/rumors/pkg/logger"
	"github.com/iagapie/rumors/pkg/mongodb"
	"github.com/iagapie/rumors/pkg/validate"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os/signal"
	"syscall"
	"time"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start API server",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return viper.BindPFlags(cmd.LocalFlags())
	},
	RunE: serve,
}

func init() {
	flagSet := serveCmd.PersistentFlags()

	flagSet.String("server.network", "tcp", "server network, ex: tcp, tcp4, tcp6, unix, unixpacket")
	flagSet.String("server.address", ":8080", "server address")

	flagSet.Int64("telegram.owner", 0, "telegram (BOT) owner id")
	flagSet.String("telegram.token", "", "telegram bot token")

	flagSet.Int("asynq.server.concurrency", 0, "how many concurrent workers to use, zero or negative for number of CPUs")
	flagSet.Int("asynq.server.group.max.size", 50, "if zero no delay limit is used")
	flagSet.Duration("asynq.server.group.max.delay", 10*time.Minute, "if zero no size limit is used")
	flagSet.Duration("asynq.server.group.grace.period", 2*time.Minute, "min 1 second")
	flagSet.String("asynq.scheduler.feed", "@every 5m", "feed importer cron spec string or can use \"@every <duration>\" to specify the interval")

	RootCmd.AddCommand(serveCmd)
}

func serve(cmd *cobra.Command, _ []string) error {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return err
	}

	logger.ZeroLogWith(cfg.Log.Level, name(cmd), cmd.Version, cfg.Log.Console, cfg.Debug)
	_ = tgbotapi.SetLogger(logger.NewBotLogger())

	mongoDB, err := mongodb.GetDB(cmd.Context(), cfg.MongoDB.URI)
	if err != nil {
		return err
	}

	roomStorage, err := storage.NewRoomStorage(cmd.Context(), mongoDB)
	if err != nil {
		return err
	}

	feedStorage, err := storage.NewFeedStorage(cmd.Context(), mongoDB)
	if err != nil {
		return err
	}

	feedItemStorage, err := storage.NewFeedItemStorage(cmd.Context(), mongoDB)
	if err != nil {
		return err
	}

	validator := validate.New()
	em := emitter.NewEmitter(10)

	redisConnOpt := asynq.RedisClientOpt{
		Network:  cfg.Redis.Network,
		Addr:     cfg.Redis.Address,
		Username: cfg.Redis.Username,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	taskApp := tasks.NewApp(cfg.Asynq, redisConnOpt)
	httpApp := http.NewApp(cfg.Debug, cfg.Server, validator)
	botApi, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return err
	}
	botApi.Debug = cfg.Debug

	listener := &events.Listener{
		BotAPI:  botApi,
		Emitter: em,
		Log:     log.Ctx(cmd.Context()).With().Str("context", "listener").Logger(),
		Owner:   cfg.Telegram.Owner,
	}

	telegramMux := asynq.NewServeMux()
	telegramMux.Handle(consts.TaskTelegramUpdate, &tasksHandlers.TelegramUpdateHandler{
		Client:  taskApp.Client(),
		Emitter: em,
		Owner:   cfg.Telegram.Owner,
	})

	roomMux := asynq.NewServeMux()
	roomMux.Handle(consts.TaskRoomSave, &tasksHandlers.RoomSaveHandler{
		Storage: roomStorage,
		Emitter: em,
	})
	roomMux.Handle(consts.TaskRoomAdd, &tasksHandlers.RoomAddLeftHandler{
		Storage: roomStorage,
		Client:  taskApp.Client(),
	})
	roomMux.Handle(consts.TaskRoomLeft, &tasksHandlers.RoomAddLeftHandler{
		Storage: roomStorage,
		Client:  taskApp.Client(),
	})
	roomMux.Handle(consts.TaskRoomView, &tasksHandlers.RoomViewHandler{
		Storage: roomStorage,
		Emitter: em,
	})

	feedMux := asynq.NewServeMux()
	feedMux.Handle(consts.TaskFeedScheduler, &tasksHandlers.FeedSchedulerHandler{
		Storage: feedStorage,
		Client:  taskApp.Client(),
	})
	feedMux.Handle(consts.TaskFeedImporter, &tasksHandlers.FeedImporterHandler{
		Client: taskApp.Client(),
	})
	feedMux.Handle(consts.TaskFeedSave, &tasksHandlers.FeedSaveHandler{
		Storage: feedStorage,
		Emitter: em,
	})
	feedMux.Handle(consts.TaskFeedAdd, &tasksHandlers.FeedAddHandler{
		Validator: validator,
		Emitter:   em,
		Client:    taskApp.Client(),
		Owner:     cfg.Telegram.Owner,
	})
	feedMux.Handle(consts.TaskFeedView, &tasksHandlers.FeedViewHandler{
		Storage: feedStorage,
		Emitter: em,
	})

	feedItemMux := asynq.NewServeMux()
	feedItemMux.Handle(consts.TaskFeedItemSave, &tasksHandlers.FeedItemSaveHandler{
		Storage: feedItemStorage,
		Client:  taskApp.Client(),
	})
	feedItemMux.Handle(consts.TaskFeedItemView, &tasksHandlers.FeedItemViewHandler{
		Storage: feedItemStorage,
		Emitter: em,
	})
	feedItemMux.Handle(consts.TaskFeedItemAggregated, &tasksHandlers.FeedItemAggregatedHandler{
		Storage: roomStorage,
		Client:  taskApp.Client(),
	})
	feedItemMux.Handle(consts.TaskFeedItemBroadcast, &tasksHandlers.FeedItemBroadcastHandler{
		Emitter: em,
	})

	mux := asynq.NewServeMux()
	mux.Use(func(handler asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			l := log.
				Ctx(ctx).
				With().
				Str("context", "tasks").
				Str("task", task.Type()).
				RawJSON("payload", task.Payload()).
				Logger()
			return handler.ProcessTask(l.WithContext(ctx), task)
		})
	})
	mux.Handle(consts.TaskTelegramPrefix, telegramMux)
	mux.Handle(consts.TaskRoomPrefix, roomMux)
	mux.Handle(consts.TaskFeedPrefix, feedMux)
	mux.Handle(consts.TaskFeedItemPrefix, feedItemMux)

	api := httpApp.Echo().Group("/api")
	{
		v1 := api.Group("/v1")
		{
			feedHandler := &httpHandlers.FeedHandler{}
			feedHandler.Register(v1)

			roomHandler := &httpHandlers.RoomHandler{}
			roomHandler.Register(v1)

			updateHandler := &httpHandlers.UpdateHandler{}
			updateHandler.Register(v1)
		}
	}

	ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT, syscall.SIGTSTP)
	defer cancel()

	listener.Start()
	defer listener.Shutdown()

	taskApp.Start(mux)
	defer taskApp.Shutdown()

	go startGetUpdates(botApi, taskApp.Client())
	defer botApi.StopReceivingUpdates()

	go httpApp.Start()
	defer httpApp.Shutdown()

	em.Fire(cmd.Context(), consts.EventAppStart)

	<-ctx.Done()

	em.Fire(context.Background(), consts.EventAppStop)

	return nil
}

func startGetUpdates(botApi *tgbotapi.BotAPI, client *asynq.Client) {
	l := log.With().Str("context", "tgbotapi").Logger()

	cfg := tgbotapi.UpdateConfig{
		Timeout: 30,
		AllowedUpdates: []string{
			"message",
			"edited_message",
			"channel_post",
			"edited_channel_post",
			"my_chat_member",
		},
	}

	for update := range botApi.GetUpdatesChan(cfg) {
		payload, err := json.Marshal(update)
		if err != nil {
			l.Error().Err(err).Msg("error due to marshal update")
			continue
		}

		taskId := asynq.TaskID(fmt.Sprintf("%s:%d", consts.TaskTelegramUpdate, update.UpdateID))
		if _, err = client.Enqueue(asynq.NewTask(consts.TaskTelegramUpdate, payload), taskId); err != nil {
			l.Error().Err(err).Msg("error due to enqueue update")
		}
	}
}
