package models

import (
	"fmt"
	"strings"
	"time"
)

type FeedItem struct {
	Id         string    `json:"id,omitempty" bson:"_id,omitempty"`
	FeedId     string    `json:"feed_id" bson:"feed_id,omitempty"`
	Title      string    `json:"title" bson:"title,omitempty"`
	Desc       string    `json:"desc,omitempty" bson:"desc,omitempty"`
	Link       string    `json:"link" bson:"link,omitempty"`
	Guid       string    `json:"guid" bson:"guid,omitempty"`
	PubDate    time.Time `json:"pub_date" bson:"pub_date,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty" bson:"created_at,omitempty"`
	Authors    []string  `json:"authors" bson:"authors,omitempty"`
	Categories []string  `json:"categories" bson:"categories,omitempty"`
}

func (i *FeedItem) Line() string {
	return fmt.Sprintf("%s - <a href=\"%s\">link</a>", i.Title, i.Link)
}

func (i *FeedItem) Info() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<a href=\"%s\">%s</a>", i.Link, i.Title))

	for n, cat := range i.Categories {
		if n == 0 {
			b.WriteString("\n")
		}
		b.WriteString(cat)
		if (n + 1) < len(i.Categories) {
			b.WriteString(", ")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(i.Desc)

	for n, cat := range i.Authors {
		if n == 0 {
			b.WriteString("\n")
		}
		b.WriteString(cat)
		if (n + 1) < len(i.Authors) {
			b.WriteString(", ")
		}
	}

	return b.String()
}
