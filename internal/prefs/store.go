// Package prefs loads and saves local settings independently of the desktop UI.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
)

// Settings holds the schedule and desktop preferences.
type Settings struct {
	breaks.Settings
	Language i18n.Language
}

func Defaults() Settings {
	return Settings{Settings: breaks.Defaults()}
}

func (s Settings) Validate() error {
	if !s.Language.Valid() {
		return fmt.Errorf("unsupported language %q", s.Language)
	}
	return s.Settings.Validate()
}

// Store whole seconds as int32 so even invalid inputs convert without overflowing time.Duration.
type fileSettings struct {
	Interval       int32         `json:"interval"`
	Short          fileRule      `json:"short"`
	Long           fileRule      `json:"long"`
	SetupConfirmed bool          `json:"setupConfirmed"`
	Language       i18n.Language `json:"language,omitempty"`
}

type fileRule struct {
	Duration int32 `json:"duration"`
	Notice   int32 `json:"notice"`
	Postpone int32 `json:"postpone"`
	Strict   bool  `json:"strict"`
}

func encode(s Settings) fileSettings {
	rule := func(r breaks.Rule) fileRule {
		return fileRule{int32(r.Duration / time.Second), int32(r.Notice / time.Second), int32(r.Postpone / time.Second), r.Strict}
	}
	return fileSettings{
		Interval:       int32(s.Interval / time.Second),
		Short:          rule(s.Short),
		Long:           rule(s.Long),
		SetupConfirmed: true,
		Language:       s.Language,
	}
}

func (f fileSettings) decode() Settings {
	rule := func(r fileRule) breaks.Rule {
		return breaks.Rule{
			Duration: time.Duration(r.Duration) * time.Second,
			Notice:   time.Duration(r.Notice) * time.Second,
			Postpone: time.Duration(r.Postpone) * time.Second,
			Strict:   r.Strict,
		}
	}
	return Settings{
		Settings: breaks.Settings{Interval: time.Duration(f.Interval) * time.Second, Short: rule(f.Short), Long: rule(f.Long)},
		Language: f.Language,
	}
}

func Path() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "pausepoint", "settings.json"), nil
}

// Load returns the settings and whether setup has been confirmed. A valid
// legacy file also counts as confirmed because it was created by an explicit save.
// A missing or invalid file returns unconfirmed defaults and is left untouched.
func Load(path string) (Settings, bool, error) {
	defaults := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, false, nil
	}
	if err != nil {
		return defaults, false, err
	}
	encoded := new(encode(defaults))
	if err := json.Unmarshal(data, &encoded); err != nil {
		return defaults, false, fmt.Errorf("invalid settings: %w", err)
	}
	if encoded == nil {
		return defaults, false, errors.New("settings must be a JSON object")
	}
	settings := encoded.decode()
	if err := settings.Validate(); err != nil {
		return defaults, false, err
	}
	return settings, encoded.SetupConfirmed, nil
}

// Save validates and writes settings, recording that setup has been confirmed.
func Save(path string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(encode(settings), "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
