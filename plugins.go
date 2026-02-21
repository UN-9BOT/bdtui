package main

import "strings"

type PluginToggles map[string]bool

func defaultPluginToggles() PluginToggles {
	return PluginToggles{
		"tmux": true,
	}
}

func normalizePluginName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (t PluginToggles) enabled(name string) bool {
	if len(t) == 0 {
		return false
	}
	v, ok := t[normalizePluginName(name)]
	if !ok {
		return false
	}
	return v
}

func clonePluginToggles(src PluginToggles) PluginToggles {
	out := make(PluginToggles, len(src))
	for k, v := range src {
		out[normalizePluginName(k)] = v
	}
	return out
}

type PluginRegistry struct {
	toggles PluginToggles
	tmux    *TmuxPlugin
}

func newPluginRegistry(cfg Config) PluginRegistry {
	toggles := defaultPluginToggles()
	for k, v := range cfg.Plugins {
		toggles[normalizePluginName(k)] = v
	}

	return PluginRegistry{
		toggles: clonePluginToggles(toggles),
		tmux:    newTmuxPlugin(toggles.enabled("tmux"), shellTmuxRunner{}),
	}
}

func (r PluginRegistry) Enabled(name string) bool {
	return r.toggles.enabled(name)
}

func (r PluginRegistry) Tmux() *TmuxPlugin {
	return r.tmux
}
