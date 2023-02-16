package task

import (
	"context"
	"fmt"
	"github.com/abadojack/whatlanggo"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/mmcdole/gofeed"
	"github.com/otiai10/opengraph/v2"
	"github.com/rumorsflow/rumors/v2/internal/entity"
	"github.com/rumorsflow/rumors/v2/internal/pubsub"
	"github.com/rumorsflow/rumors/v2/internal/repository"
	"github.com/rumorsflow/rumors/v2/internal/repository/db"
	"github.com/rumorsflow/rumors/v2/pkg/conv"
	"github.com/rumorsflow/rumors/v2/pkg/errs"
	"github.com/rumorsflow/rumors/v2/pkg/strutil"
	"golang.org/x/exp/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minShortDesc = 20
	maxShortDesc = 500
	userAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/109.0"
)

type HandlerJobFeed struct {
	logger      *slog.Logger
	publisher   *pubsub.Publisher
	feedRepo    repository.ReadRepository[*entity.Feed]
	articleRepo repository.ReadWriteRepository[*entity.Article]
}

func (h *HandlerJobFeed) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if task.Payload() == nil {
		h.logger.Warn("task payload is empty")
		return nil
	}

	id, err := uuid.Parse(conv.BytesToString(task.Payload()))
	if err != nil {
		h.logger.Error("error due to parse uuid", err, "payload", task.Payload())
		return nil
	}

	feed, err := h.feedRepo.FindByID(ctx, id)
	if err != nil {
		if errs.Is(err, repository.ErrEntityNotFound) {
			h.logger.Error("error due to find feed", err, "id", id)
			return nil
		}
		return errs.E(OpServerProcessTask, id, "error due to find feed", err)
	}

	parsed, err := h.parseFeed(ctx, feed.Link)
	if err != nil {
		if !errs.IsCanceledOrDeadline(err) {
			h.logger.Error("error due to parse feed", err, "id", feed.ID)
		}
		return nil
	}

	lastIndex, err := h.findLastIndex(ctx, parsed.Items)
	if err != nil {
		return err
	}

	if lastIndex > -1 {
		if n := len(parsed.Items) - lastIndex - 1; n > 0 {
			items := make([]*gofeed.Item, len(parsed.Items)-lastIndex-1)
			for i := 0; i <= lastIndex; i++ {
				parsed.Items[i] = nil
			}
			copy(items, parsed.Items[lastIndex+1:])
			parsed.Items = items
		} else {
			return nil
		}
	}

	for _, item := range parsed.Items {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		h.processItem(ctx, feed, item)
	}

	return nil
}

func (h *HandlerJobFeed) processItem(ctx context.Context, feed *entity.Feed, item *gofeed.Item) {
	og, err := h.parseOpengraphMeta(ctx, item.Link)
	if err != nil {
		if !errs.IsCanceledOrDeadline(err) {
			h.logger.Error("error due to parse feed item's link", errs.E(OpServerProcessTask, err), "item", item)
		}
		return
	}

	if item.Description == "" {
		if item.Description = item.Content; item.Description == "" {
			item.Description = og.Description
		}
	}

	var lang, shortDesc string

	if shortDesc = strutil.StripHTMLTags(og.Description); utf8.RuneCountInString(shortDesc) < minShortDesc {
		if shortDesc = strutil.StripHTMLTags(item.Description); utf8.RuneCountInString(shortDesc) > maxShortDesc {
			shortDesc = string([]rune(shortDesc)[:maxShortDesc-3])
			shortDesc = strings.TrimSuffix(shortDesc, ".") + "..."
		}
	}

	if item.Title = strutil.StripHTMLTags(item.Title); item.Title == "" {
		if item.Title = strutil.StripHTMLTags(og.Title); item.Title == "" {
			if item.Title = shortDesc; utf8.RuneCountInString(item.Title) > 100 {
				item.Title = strings.TrimSuffix(string([]rune(item.Title)[:97]), ".") + "..."
			}
		}
	}

	if item.Title == "" {
		h.logger.Warn("article title not found", "feed", item, "og", og)
		return
	}

	if lang = whatlanggo.DetectLang(item.Title + " " + shortDesc + " " + item.Description).Iso6391(); lang == "" || lang == "xx" {
		if len(feed.Languages) > 0 {
			lang = feed.Languages[0]
		} else {
			h.logger.Warn("feed item's lang not detected", "item", item)
			return
		}
	}

	article := &entity.Article{
		ID:       uuid.New(),
		SourceID: feed.ID,
		Source:   entity.FeedSource,
		Lang:     lang,
		Title:    item.Title,
		Link:     item.Link,
		PubDate:  *item.PublishedParsed,
	}

	if utf8.RuneCountInString(shortDesc) >= 50 {
		article.SetShortDesc(shortDesc)
	}

	if utf8.RuneCountInString(item.Description) >= 50 {
		article.SetLongDesc(item.Description)
	}

	authors := make([]string, 0, len(item.Authors))
	for _, author := range item.Authors {
		if author != nil {
			if name := strutil.StripHTMLTags(author.Name); name != "" {
				authors = append(authors, name)
			}
		}
	}

	if len(authors) > 0 {
		article.SetAuthors(authors)
	}

	categories := make([]string, 0, len(item.Categories))
	for _, category := range item.Categories {
		if category = strutil.StripHTMLTags(category); category != "" {
			categories = append(categories, category)
		}
	}

	if len(categories) > 0 {
		article.SetCategories(categories)
	}

	media := make([]entity.Media, 0, len(og.Image)+len(og.Video)+len(og.Audio))
	for _, i := range og.Image {
		media = append(media, entity.Media{URL: i.URL, Type: entity.ImageType, Meta: map[string]any{
			"width":  i.Width,
			"height": i.Height,
			"alt":    i.Alt,
		}})
	}
	for _, i := range og.Video {
		media = append(media, entity.Media{URL: i.URL, Type: entity.VideoType, Meta: map[string]any{
			"width":    i.Width,
			"height":   i.Height,
			"duration": i.Duration,
		}})
	}
	for _, i := range og.Audio {
		media = append(media, entity.Media{URL: i.URL, Type: entity.AudioType})
	}

	if len(media) > 0 {
		article.SetMedia(media)
	}

	h.saveArticle(ctx, article)
}

func (h *HandlerJobFeed) parseFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	p := gofeed.NewParser()
	p.UserAgent = userAgent
	parsed, err := p.ParseURLWithContext(link, ctx)
	if err != nil {
		return nil, errs.E(OpServerParseFeed, err)
	}

	items := make([]*gofeed.Item, 0, len(parsed.Items))

	for i, item := range parsed.Items {
		if item == nil {
			continue
		}

		if _, err = url.ParseRequestURI(item.GUID); err == nil {
			item.Link = item.GUID
		} else {
			if item.Link == "" && len(item.Links) > 0 {
				item.Link = item.Links[0]
			}

			if _, err = url.ParseRequestURI(item.Link); err != nil {
				parsed.Items[i] = nil
				continue
			}
		}

		if item.PublishedParsed == nil {
			if item.UpdatedParsed != nil {
				item.PublishedParsed = item.UpdatedParsed
			} else {
				now := time.Now()
				item.PublishedParsed = &now
			}
		}

		items = append(items, item)
	}

	parsed.Items = items

	sort.Slice(parsed.Items, func(i, j int) bool {
		return parsed.Items[i].PublishedParsed.Before(*parsed.Items[j].PublishedParsed)
	})

	h.logger.Debug("feed link parsed", "items", parsed.Items)

	return parsed, err
}

func (h *HandlerJobFeed) saveArticle(ctx context.Context, article *entity.Article) {
	if err := h.articleRepo.Save(ctx, article); err != nil {
		if errs.IsCanceledOrDeadline(err) {
			return
		}

		if errs.Is(err, repository.ErrDuplicateKey) {
			h.logger.Debug("error due to save article, duplicate key", "article", article)
		} else {
			h.logger.Error("error due to save article", err, "article", article)
		}
		return
	}

	h.logger.Debug("article saved", "article", article)

	h.publisher.Articles(ctx, []pubsub.Article{pubsub.FromEntity(article)})
}

func (h *HandlerJobFeed) findLastIndex(ctx context.Context, items []*gofeed.Item) (int, error) {
	seen := make(map[string]int, len(items))
	links := make([]string, len(items))

	for i, item := range items {
		seen[item.Link] = i
		links[i] = item.Link
	}

	query := fmt.Sprintf("sort=-pub_date&field.0.0=link&cond.0.0=in&value.0.0=%s", strings.Join(links, ","))
	criteria := db.BuildCriteria(query).SetSize(int64(len(links)))

	iter, err := h.articleRepo.FindIter(ctx, criteria)
	if err != nil {
		return -1, errs.E(OpServerProcessTask, "error due to find article last index", err)
	}

	defer func() {
		_ = iter.Close(context.Background())
	}()

	for iter.Next(ctx) {
		article := iter.Entity()

		if i, ok := seen[article.Link]; ok {
			return i, nil
		}
	}

	return -1, nil
}

func (h *HandlerJobFeed) parseOpengraphMeta(ctx context.Context, link string) (*opengraph.OpenGraph, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	og, err := openGraphFetch(ctx, link)
	if err != nil {
		return nil, errs.E(OpServerParseArticle, err)
	}

	h.logger.Debug("article link parsed", "article", og)

	return og, nil
}

func openGraphFetch(ctx context.Context, url string) (*opengraph.OpenGraph, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		return nil, errs.New("content type must be text/html")
	}

	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("open graph error due to request %s with response status code %d", url, res.StatusCode)
	}

	og := opengraph.New(url)

	if err = og.Parse(res.Body); err != nil {
		return nil, err
	}

	return og, nil
}
