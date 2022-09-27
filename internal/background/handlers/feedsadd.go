package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iagapie/rumors/internal/models"
	"github.com/iagapie/rumors/internal/notifications"
	"github.com/iagapie/rumors/internal/storage"
	"github.com/mmcdole/gofeed"
	"github.com/rs/zerolog"
	"net/url"
	"strings"
	"time"
)

type FeedsAddHandler struct {
	Notification notifications.Notification
	FeedStorage  storage.FeedStorage
	Log          *zerolog.Logger
	Owner        int64
}

func (h *FeedsAddHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var message tgbotapi.Message

	if err := json.Unmarshal(task.Payload(), &message); err != nil {
		h.Log.Error().Err(err).Msg("")
		return nil
	}

	link := strings.TrimSpace(message.CommandArguments())
	if link == "" {
		h.Notification.Error(message.Chat.ID, "URL is required\n/add <feed url>")
		return nil
	}

	u, err := url.Parse(link)
	if err != nil {
		h.Notification.Err(message.Chat.ID, err)
		return nil
	}

	pCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	feed, err := gofeed.NewParser().ParseURLWithContext(link, pCtx)
	if err != nil {
		h.Notification.Err(message.Chat.ID, err)
		return nil
	}
	cancel()

	by := message.Chat.ID
	if message.From != nil {
		by = message.From.ID
	}

	if _, err = h.FeedStorage.FindByLink(ctx, feed.FeedLink); err == nil {
		h.Notification.Error(message.Chat.ID, fmt.Sprintf("The specified URL %s by %d already exists", link, by))
		return nil
	}

	model := models.Feed{
		Id:        uuid.NewString(),
		By:        by,
		Host:      strings.ToLower(strings.ReplaceAll(u.Hostname(), "www.", "")),
		Title:     feed.Title,
		Link:      feed.FeedLink,
		Enabled:   by == h.Owner,
		CreatedAt: time.Now().UTC(),
	}

	if err = h.FeedStorage.Save(ctx, model); err != nil {
		h.Notification.Err(nil, err)
		return nil
	}

	if model.By != h.Owner {
		h.Notification.Success(message.Chat.ID, fmt.Sprintf("The specified URL %s by %d was added successfully and waiting to pass moderation", link, by))
	}

	h.Notification.Send(nil, model.Info())

	return nil
}
