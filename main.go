package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ach1000/gator/internal/config"
	"github.com/ach1000/gator/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type state struct {
	cfg  *config.Config
	db   UserStore
	cmds *commands
}

type UserStore interface {
	GetUser(context.Context, string) (database.User, error)
	GetUsers(context.Context) ([]database.User, error)
	CreateUser(context.Context, database.CreateUserParams) (database.User, error)
	CreateFeed(context.Context, database.CreateFeedParams) (database.CreateFeedRow, error)
	GetAllFeeds(context.Context) ([]database.GetAllFeedsRow, error)
	CreateFeedFollow(context.Context, database.CreateFeedFollowParams) (database.CreateFeedFollowRow, error)
	GetFeedByURL(context.Context, string) (database.GetFeedByURLRow, error)
	GetFeedFollowsForUser(context.Context, uuid.UUID) ([]database.GetFeedFollowsForUserRow, error)
	DeleteFeedFollow(context.Context, database.DeleteFeedFollowParams) error
	DeleteUsers(context.Context) error
	GetNextFeedToFetch(context.Context) (database.Feed, error)
	MarkFeedFetched(context.Context, database.MarkFeedFetchedParams) error
}

type command struct {
	name string
	args []string
}

type commands struct {
	handlers     map[string]func(*state, command) error
	descriptions map[string]string
}

func (c *commands) register(name string, description string, f func(*state, command) error) {
	if c.handlers == nil {
		c.handlers = make(map[string]func(*state, command) error)
		c.descriptions = make(map[string]string)
	}
	c.handlers[name] = f
	c.descriptions[name] = description
}

func (c *commands) run(s *state, cmd command) error {
	f, ok := c.handlers[cmd.name]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmd.name)
	}
	return f(s, cmd)
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("username required")
	}
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	username := cmd.args[0]
	_, err := s.db.GetUser(context.Background(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %q does not exist", username)
		}
		return err
	}

	if err := s.cfg.SetUser(username); err != nil {
		return err
	}

	fmt.Printf("current user set to %s\n", username)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("username required")
	}
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	name := cmd.args[0]
	_, err := s.db.GetUser(context.Background(), name)
	if err == nil {
		return fmt.Errorf("user %q already exists", name)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := time.Now().UTC()
	user, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
	})
	if err != nil {
		return err
	}

	if err := s.cfg.SetUser(name); err != nil {
		return err
	}

	fmt.Printf("created user: %s\n", name)
	log.Printf("user data: %#v\n", user)
	return nil
}

func handlerReset(s *state, _ command) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := s.db.DeleteUsers(context.Background()); err != nil {
		return err
	}

	fmt.Println("database reset: all users deleted")
	return nil
}

func handlerUsers(s *state, _ command) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return err
	}

	current := s.cfg.CurrentUserName
	for _, u := range users {
		if u.Name == current {
			fmt.Printf("* %s (current)\n", u.Name)
		} else {
			fmt.Printf("* %s\n", u.Name)
		}
	}

	return nil
}

func handlerHelp(s *state, _ command) error {
	if s.cmds == nil {
		return fmt.Errorf("commands registry not initialized")
	}
	fmt.Println("Available commands:")
	for name, desc := range s.cmds.descriptions {
		fmt.Printf("  %-15s %s\n", name, desc)
	}
	return nil
}

func middlewareLoggedIn(handler func(*state, command, database.User) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		if s.cfg.CurrentUserName == "" {
			return fmt.Errorf("no current user set")
		}
		if s.db == nil {
			return fmt.Errorf("database not initialized")
		}

		user, err := s.db.GetUser(context.Background(), s.cfg.CurrentUserName)
		if err != nil {
			return err
		}

		return handler(s, cmd, user)
	}
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("name and url required")
	}

	feedName := cmd.args[0]
	feedURL := cmd.args[1]
	now := time.Now().UTC()

	feed, err := s.db.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      feedName,
		Url:       feedURL,
		UserID:    user.ID,
	})
	if err != nil {
		return err
	}

	// Automatically create a feed follow for the current user
	_, err = s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("created feed: %+v\n", feed)
	return nil
}

func handlerFeeds(s *state, _ command) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	feeds, err := s.db.GetAllFeeds(context.Background())
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		fmt.Printf("* %s (by %s) - %s\n", feed.Name, feed.UserName, feed.Url)
	}

	return nil
}

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("url required")
	}

	feedURL := cmd.args[0]

	feed, err := s.db.GetFeedByURL(context.Background(), feedURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("feed with url %q does not exist", feedURL)
		}
		return err
	}

	now := time.Now().UTC()
	followRow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Now following %s (by %s)\n", followRow.FeedName, followRow.UserName)
	return nil
}

func handlerFollowing(s *state, _ command, user database.User) error {
	follows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}

	if len(follows) == 0 {
		fmt.Println("No feeds are being followed")
		return nil
	}

	for _, follow := range follows {
		fmt.Printf("* %s\n", follow.FeedName)
	}

	return nil
}

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("url required")
	}

	feedURL := cmd.args[0]

	feed, err := s.db.GetFeedByURL(context.Background(), feedURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("feed with url %q does not exist", feedURL)
		}
		return err
	}

	err = s.db.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Unfollowed %s\n", feed.Name)
	return nil
}

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}

	if cfg.DBUrl == "" {
		log.Fatal("db_url is not configured in config file")
	}

	sqlDB, err := sql.Open("postgres", cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	dbQueries := database.New(sqlDB)

	cmds := &commands{}
	cmds.register("help", "Show available commands", handlerHelp)
	cmds.register("login", "Set the current user", handlerLogin)
	cmds.register("register", "Create a new user", handlerRegister)
	cmds.register("reset", "Delete all users and feeds", handlerReset)
	cmds.register("users", "List all users", handlerUsers)
	cmds.register("addfeed", "Add a feed for the current user", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", "List all feeds with usernames", handlerFeeds)
	cmds.register("follow", "Follow a feed by URL", middlewareLoggedIn(handlerFollow))
	cmds.register("following", "List all feeds the current user is following", middlewareLoggedIn(handlerFollowing))
	cmds.register("unfollow", "Unfollow a feed by URL", middlewareLoggedIn(handlerUnfollow))
	cmds.register("agg", "Aggregate RSS feeds in a loop with time interval (e.g., 1m, 1h)", handlerAgg)

	s := &state{cfg: cfg, db: dbQueries, cmds: cmds}

	args := os.Args
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "not enough arguments")
		os.Exit(1)
	}

	cmd := command{name: args[1], args: args[2:]}
	if err := cmds.run(s, cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
