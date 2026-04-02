package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"time"

	"github.com/ach1000/gator/internal/database"
	"github.com/google/uuid"
)

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gator")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed RSSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}

	feed.Channel.Title = html.UnescapeString(feed.Channel.Title)
	feed.Channel.Description = html.UnescapeString(feed.Channel.Description)

	for i := range feed.Channel.Item {
		feed.Channel.Item[i].Title = html.UnescapeString(feed.Channel.Item[i].Title)
		feed.Channel.Item[i].Description = html.UnescapeString(feed.Channel.Item[i].Description)
	}

	return &feed, nil
}

func scrapeFeeds(s *state) error {
	nextFeed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		return err
	}

	fmt.Printf("Fetching feed: %s\n", nextFeed.Url)

	rssFeed, err := fetchFeed(context.Background(), nextFeed.Url)
	if err != nil {
		return err
	}

	for _, item := range rssFeed.Channel.Item {
		fmt.Printf(" * %s\n", item.Title)

		// Parse published_at time
		var publishedAt sql.NullTime
		pubTime, err := time.Parse(time.RFC1123Z, item.PubDate)
		if err != nil {
			// Try alternative format
			pubTime, err = time.Parse("Mon, 02 Jan 2006 15:04:05 MST", item.PubDate)
			if err != nil {
				// If parsing fails, leave as NULL
				publishedAt = sql.NullTime{Valid: false}
			} else {
				publishedAt = sql.NullTime{Time: pubTime, Valid: true}
			}
		} else {
			publishedAt = sql.NullTime{Time: pubTime, Valid: true}
		}

		now := time.Now().UTC()
		_, err = s.db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   now,
			UpdatedAt:   now,
			Title:       item.Title,
			Url:         item.Link,
			Description: sql.NullString{String: item.Description, Valid: item.Description != ""},
			PublishedAt: publishedAt,
			FeedID:      nextFeed.ID,
		})
		if err != nil {
			// Check if it's a unique constraint violation (duplicate URL)
			if err.Error() == "pq: duplicate key value violates unique constraint \"posts_url_key\"" {
				// Ignore duplicate posts
				continue
			}
			// Log other errors but continue
			fmt.Printf("Error saving post %q: %v\n", item.Title, err)
		}
	}

	now := time.Now().UTC()
	err = s.db.MarkFeedFetched(context.Background(), database.MarkFeedFetchedParams{
		LastFetchedAt: sql.NullTime{Time: now, Valid: true},
		ID:            nextFeed.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("time_between_reqs required")
	}

	timeBetweenRequestsStr := cmd.args[0]
	timeBetweenRequests, err := time.ParseDuration(timeBetweenRequestsStr)
	if err != nil {
		return err
	}

	fmt.Printf("Collecting feeds every %v\n", timeBetweenRequests)

	ticker := time.NewTicker(timeBetweenRequests)
	defer ticker.Stop()

	for {
		err := scrapeFeeds(s)
		if err != nil {
			fmt.Printf("Error scraping feeds: %v\n", err)
		}

		<-ticker.C
	}
}
