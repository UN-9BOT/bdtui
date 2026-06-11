package app

import "strings"

type PluginToggles map[string]bool

func defaultPluginToggles() PluginToggles {
	return PluginToggles{
		"herdr": true,
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
	Toggles     PluginToggles
	HerdrPlugin *HerdrPlugin
}

func newPluginRegistry(cfg Config) PluginRegistry {
	toggles := defaultPluginToggles()
	for k, v := range cfg.Plugins {
		toggles[normalizePluginName(k)] = v
	}

	return PluginRegistry{
		Toggles:     clonePluginToggles(toggles),
		HerdrPlugin: newHerdrPlugin(toggles.enabled("herdr"), shellHerdrRunner{}),
	}
}

func (r PluginRegistry) Enabled(name string) bool {
	return r.Toggles.enabled(name)
}

func (r PluginRegistry) Herdr() *HerdrPlugin {
	return r.HerdrPlugin
}
