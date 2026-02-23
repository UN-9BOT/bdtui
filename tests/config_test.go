package bdtui_test

import "testing"

func TestParseConfig_DefaultPlugins(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if !cfg.Plugins.Enabled("tmux") {
		t.Fatalf("expected tmux plugin enabled by default")
	}
}

func TestParseConfig_PluginOverrides(t *testing.T) {
	cfg, err := parseConfig([]string{"--plugins=-tmux,custom"})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Plugins.Enabled("tmux") {
		t.Fatalf("expected tmux plugin disabled")
	}
	if !cfg.Plugins.Enabled("custom") {
		t.Fatalf("expected custom plugin enabled")
	}
}

func TestParsePluginToggles_EmptyTokenError(t *testing.T) {
	_, err := parsePluginToggles("-")
	if err == nil {
		t.Fatalf("expected error for empty plugin token")
	}
}
