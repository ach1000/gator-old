package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ach1000/gator/internal/config"
	"github.com/ach1000/gator/internal/database"
)

type mockStore struct {
	users map[string]database.User
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

func TestCommandsRegisterAndRun(t *testing.T) {
	cmds := &commands{}
	received := false
	cmds.register("test", func(s *state, cmd command) error {
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

	if err := handlerRegister(s, command{name: "register", args: []string{"lane"}}); err != nil {
		t.Fatalf("handlerRegister error: %v", err)
	}

	if _, ok := store.users["lane"]; !ok {
		t.Fatal("expected user lane to be created")
	}

	if err := handlerRegister(s, command{name: "register", args: []string{"lane"}}); err == nil {
		t.Fatal("expected error on duplicate user")
	}
}
