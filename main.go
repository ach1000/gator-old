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
	cfg *config.Config
	db  UserStore
}

type UserStore interface {
	GetUser(context.Context, string) (database.User, error)
	GetUsers(context.Context) ([]database.User, error)
	CreateUser(context.Context, database.CreateUserParams) (database.User, error)
	DeleteUsers(context.Context) error
}

type command struct {
	name string
	args []string
}

type commands struct {
	handlers map[string]func(*state, command) error
}

func (c *commands) register(name string, f func(*state, command) error) {
	if c.handlers == nil {
		c.handlers = make(map[string]func(*state, command) error)
	}
	c.handlers[name] = f
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

	s := &state{cfg: cfg, db: dbQueries}
	cmds := &commands{}
	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerUsers)

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

