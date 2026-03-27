package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ach1000/gator/internal/config"
)

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

	s := &state{cfg: cfg}
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
}
