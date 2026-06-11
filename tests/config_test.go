package bdtui_test

import "testing"

func TestParseConfig_DefaultPlugins(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if !cfg.Plugins.Enabled("herdr") {
		t.Fatalf("expected herdr plugin enabled by default")
	}
}

func TestParseConfig_PluginOverrides(t *testing.T) {
	cfg, err := parseConfig([]string{"--plugins=-herdr,custom"})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Plugins.Enabled("herdr") {
		t.Fatalf("expected herdr plugin disabled")
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
