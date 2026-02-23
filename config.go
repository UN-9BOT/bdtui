package bdtui

import (
	"errors"
	"flag"
	"io"
	"strings"
)

type Config struct {
	BeadsDir string
	NoWatch  bool
	Plugins  PluginToggles
}

func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("bdtui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg Config
	var pluginsRaw string
	fs.StringVar(&cfg.BeadsDir, "beads-dir", "", "Path to .beads directory")
	fs.BoolVar(&cfg.NoWatch, "no-watch", false, "Disable periodic auto-reload")
	fs.StringVar(&pluginsRaw, "plugins", "", "Plugin toggles CSV (e.g. tmux,-tmux)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, errors.New("unexpected positional arguments")
	}

	plugins, err := parsePluginToggles(pluginsRaw)
	if err != nil {
		return Config{}, err
	}
	cfg.Plugins = plugins

	return cfg, nil
}

func parsePluginToggles(raw string) (PluginToggles, error) {
	toggles := defaultPluginToggles()
	value := strings.TrimSpace(raw)
	if value == "" {
		return toggles, nil
	}

	for _, part := range strings.Split(value, ",") {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}

		enabled := true
		name := token
		if strings.HasPrefix(token, "-") {
			enabled = false
			name = strings.TrimSpace(strings.TrimPrefix(token, "-"))
		}
		if name == "" {
			return nil, errors.New("Plugins: empty plugin token")
		}

		toggles[normalizePluginName(name)] = enabled
	}

	return toggles, nil
}
