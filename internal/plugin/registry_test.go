package plugin_test

import (
	"errors"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/plugin"
	"github.com/spf13/cobra"
)

// fakePlugin is a test double for Plugin.
type fakePlugin struct {
	name        string
	initCalled  bool
	initErr     error
}

func (f *fakePlugin) Name() string                  { return f.name }
func (f *fakePlugin) Version() string               { return "0.0.1" }
func (f *fakePlugin) Description() string           { return "fake plugin" }
func (f *fakePlugin) Commands() []*cobra.Command    { return nil }
func (f *fakePlugin) Init(cfg *config.Config) error {
	f.initCalled = true
	return f.initErr
}

func TestRegister(t *testing.T) {
	r := plugin.NewRegistry()
	p := &fakePlugin{name: "test"}

	if err := r.Register(p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("expected plugin to be found")
	}
	if got.Name() != "test" {
		t.Errorf("got name %q, want %q", got.Name(), "test")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := plugin.NewRegistry()
	p := &fakePlugin{name: "dup"}

	if err := r.Register(p); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(p); err == nil {
		t.Fatal("expected error on duplicate register, got nil")
	}
}

func TestInitAll(t *testing.T) {
	r := plugin.NewRegistry()
	p1 := &fakePlugin{name: "alpha"}
	p2 := &fakePlugin{name: "beta"}

	_ = r.Register(p1)
	_ = r.Register(p2)

	cfg := &config.Config{}
	if err := r.InitAll(cfg, []string{"alpha"}); err != nil {
		t.Fatalf("InitAll: %v", err)
	}

	if !p1.initCalled {
		t.Error("expected alpha to be initialized")
	}
	if p2.initCalled {
		t.Error("expected beta to NOT be initialized")
	}
}

func TestInitAllEmpty(t *testing.T) {
	r := plugin.NewRegistry()
	p := &fakePlugin{name: "gamma"}
	_ = r.Register(p)

	cfg := &config.Config{}
	if err := r.InitAll(cfg, nil); err != nil {
		t.Fatalf("InitAll with empty list: %v", err)
	}
	if p.initCalled {
		t.Error("expected gamma to NOT be initialized when list is empty")
	}
}

func TestInitAllError(t *testing.T) {
	r := plugin.NewRegistry()
	p := &fakePlugin{name: "bad", initErr: errors.New("init failed")}
	_ = r.Register(p)

	cfg := &config.Config{}
	if err := r.InitAll(cfg, []string{"bad"}); err == nil {
		t.Fatal("expected error from init, got nil")
	}
}
