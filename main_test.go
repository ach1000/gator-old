package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ach1000/gator/internal/config"
	"github.com/ach1000/gator/internal/database"
	"github.com/google/uuid"
)

type mockStore struct {
	users map[string]database.User
	feeds map[string]database.Feed
}

func (m *mockStore) GetUser(_ context.Context, name string) (database.User, error) {
	if u, ok := m.users[name]; ok {
		return u, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (m *mockStore) CreateUser(_ context.Context, params database.CreateUserParams) (database.User, error) {
	if m.users == nil {
		m.users = map[string]database.User{}
	}
	if _, exists := m.users[params.Name]; exists {
		return database.User{}, fmt.Errorf("user exists")
	}
	user := database.User{ID: params.ID, CreatedAt: params.CreatedAt, UpdatedAt: params.UpdatedAt, Name: params.Name}
	m.users[params.Name] = user
	return user, nil
}

func (m *mockStore) CreateFeed(_ context.Context, params database.CreateFeedParams) (database.Feed, error) {
	if m.feeds == nil {
		m.feeds = map[string]database.Feed{}
	}
	if _, exists := m.feeds[params.Url]; exists {
		return database.Feed{}, fmt.Errorf("feed url already exists")
	}
	feed := database.Feed{ID: params.ID, CreatedAt: params.CreatedAt, UpdatedAt: params.UpdatedAt, Name: params.Name, Url: params.Url, UserID: params.UserID}
	m.feeds[params.Url] = feed
	return feed, nil
}

func (m *mockStore) GetAllFeeds(_ context.Context) ([]database.GetAllFeedsRow, error) {
	rows := make([]database.GetAllFeedsRow, 0, len(m.feeds))
	for _, feed := range m.feeds {
		userName := ""
		for _, user := range m.users {
			if user.ID == feed.UserID {
				userName = user.Name
				break
			}
		}
		rows = append(rows, database.GetAllFeedsRow{
			ID:        feed.ID,
			CreatedAt: feed.CreatedAt,
			UpdatedAt: feed.UpdatedAt,
			Name:      feed.Name,
			Url:       feed.Url,
			UserID:    feed.UserID,
			UserName:  userName,
		})
	}
	return rows, nil
}

func (m *mockStore) DeleteUsers(_ context.Context) error {
	m.users = map[string]database.User{}
	return nil
}

func (m *mockStore) GetUsers(_ context.Context) ([]database.User, error) {
	users := make([]database.User, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, u)
	}
	// Sort to match SQL order by name if necessary in non-mock.
	// We will rely on map iteration ambiguity not affecting structural expectations for now.
	return users, nil
}

func TestCommandsRegisterAndRun(t *testing.T) {
	cmds := &commands{}
	received := false
	cmds.register("test", "test command", func(s *state, cmd command) error {
		received = true
		return nil
	})

	err := cmds.run(&state{}, command{name: "test", args: nil})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !received {
		t.Fatal("expected handler to run")
	}

	err = cmds.run(&state{}, command{name: "unknown", args: nil})
	if err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestHandlerLogin(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": ""
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	store := &mockStore{users: map[string]database.User{"alice": {Name: "alice"}}}
	s := &state{cfg: cfg, db: store}
	if err := handlerLogin(s, command{name: "login", args: []string{"alice"}}); err != nil {
		t.Fatalf("handlerLogin error: %v", err)
	}

	cfg2, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read after login error: %v", err)
	}
	if cfg2.CurrentUserName != "alice" {
		t.Fatalf("expected current user alice, got %q", cfg2.CurrentUserName)
	}
	if cfg2.DBUrl != "sqlite://mydb" {
		t.Fatalf("expected DBUrl preserved, got %q", cfg2.DBUrl)
	}

	if err := handlerLogin(s, command{name: "login", args: nil}); err == nil {
		t.Fatal("expected username required error")
	}

	if err := handlerLogin(s, command{name: "login", args: []string{"bob"}}); err == nil {
		t.Fatal("expected non-existing user error")
	}
}

func TestHandlerRegister(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": ""
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	store := &mockStore{users: map[string]database.User{}}
	s := &state{cfg: cfg, db: store}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	if err := handlerRegister(s, command{name: "register", args: []string{"lane"}}); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("handlerRegister error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if !bytes.Contains(buf.Bytes(), []byte("created user: lane")) {
		t.Fatalf("expected output to mention created user, got %q", buf.String())
	}

	if _, ok := store.users["lane"]; !ok {
		t.Fatal("expected user lane to be created")
	}

	if err := handlerRegister(s, command{name: "register", args: []string{"lane"}}); err == nil {
		t.Fatal("expected error on duplicate user")
	}
}

func TestHandlerReset(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": ""
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	store := &mockStore{users: map[string]database.User{"a": {Name: "a"}, "b": {Name: "b"}}}
	s := &state{cfg: cfg, db: store}

	if err := handlerReset(s, command{name: "reset"}); err != nil {
		t.Fatalf("handlerReset error: %v", err)
	}

	if len(store.users) != 0 {
		t.Fatalf("expected no users after reset, got %d", len(store.users))
	}
}

func TestHandlerUsers(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": "allan"
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	store := &mockStore{users: map[string]database.User{"lane": {Name: "lane"}, "allan": {Name: "allan"}, "hunter": {Name: "hunter"}}}
	s := &state{cfg: cfg, db: store}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	if err := handlerUsers(s, command{name: "users"}); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("handlerUsers error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte("* allan (current)")) {
		t.Fatalf("expected current marker in users output, got %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("* lane")) || !bytes.Contains(buf.Bytes(), []byte("* hunter")) {
		t.Fatalf("expected all users listed, got %q", out)
	}
}

func TestHandlerAddFeed(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": "alice"
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	store := &mockStore{users: map[string]database.User{"alice": {ID: uuid.New(), Name: "alice"}}}
	s := &state{cfg: cfg, db: store}

	if err := handlerAddFeed(s, command{name: "addfeed", args: []string{"The Boot.dev Blog", "https://blog.boot.dev/rss"}}); err != nil {
		t.Fatalf("handlerAddFeed error: %v", err)
	}

	feed, ok := store.feeds["https://blog.boot.dev/rss"]
	if !ok {
		t.Fatal("expected feed to be stored")
	}
	if feed.Name != "The Boot.dev Blog" || feed.UserID != store.users["alice"].ID {
		t.Fatalf("unexpected feed data: %+v", feed)
	}
}

func TestHandlerFeeds(t *testing.T) {
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)

	tmp := t.TempDir()
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("failed set HOME: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".gatorconfig.json")
	data := []byte(`{
  "db_url": "sqlite://mydb",
  "current_user_name": ""
}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed write initial config: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("config.Read error: %v", err)
	}

	alice := database.User{ID: uuid.New(), Name: "alice"}
	store := &mockStore{
		users: map[string]database.User{"alice": alice},
		feeds: map[string]database.Feed{
			"https://blog.boot.dev/rss": {
				ID:     uuid.New(),
				Name:   "The Boot.dev Blog",
				Url:    "https://blog.boot.dev/rss",
				UserID: alice.ID,
			},
		},
	}
	s := &state{cfg: cfg, db: store}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	os.Stdout = w

	if err := handlerFeeds(s, command{name: "feeds"}); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("handlerFeeds error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte("The Boot.dev Blog")) {
		t.Fatalf("expected feed name in output, got %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("alice")) {
		t.Fatalf("expected user name in output, got %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("https://blog.boot.dev/rss")) {
		t.Fatalf("expected feed URL in output, got %q", out)
	}
}

func TestFetchFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test &amp; Feed</title>
    <link>https://example.com</link>
    <description>Sample &lt;RSS&gt; feed</description>
    <item>
      <title>Article &amp; One</title>
      <link>https://example.com/article1</link>
      <description>First &quot;entry&quot;</description>
      <pubDate>Mon, 06 Sep 2021 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Article &amp; Two</title>
      <link>https://example.com/article2</link>
      <description>Second &lt;entry&gt;</description>
      <pubDate>Tue, 07 Sep 2021 14:30:00 GMT</pubDate>
    </item>
  </channel>
</rss>`))
	}))
	defer srv.Close()

	feed, err := fetchFeed(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchFeed error: %v", err)
	}

	if feed.Channel.Title != "Test & Feed" {
		t.Fatalf("unexpected channel title: %q", feed.Channel.Title)
	}
	if feed.Channel.Description != "Sample <RSS> feed" {
		t.Fatalf("unexpected channel description: %q", feed.Channel.Description)
	}
	if len(feed.Channel.Item) != 2 {
		t.Fatalf("unexpected item count: %d", len(feed.Channel.Item))
	}
	if feed.Channel.Item[0].Title != "Article & One" {
		t.Fatalf("unexpected first item title: %q", feed.Channel.Item[0].Title)
	}
	if feed.Channel.Item[1].Description != "Second <entry>" {
		t.Fatalf("unexpected second item description: %q", feed.Channel.Item[1].Description)
	}
}
