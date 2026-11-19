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
	Language        i18n.Language
	Theme           Theme
	PreviewShortcut Shortcut
}

// Theme is the appearance preference. Its zero value selects the default dark palette.
type Theme string

const (
	ThemeDark   Theme = ""
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
)

// Shortcut describes the window-local preview shortcut. The platform's primary
// shortcut modifier is implicit. The zero value selects the default shortcut.
type Shortcut struct {
	Key   string `json:"key"`
	Shift bool   `json:"shift,omitempty"`
	Alt   bool   `json:"alt,omitempty"`
}

func (s Shortcut) Valid() bool {
	if s == (Shortcut{}) {
		return true
	}
	return (s.Shift || s.Alt) && len(s.Key) == 1 &&
		(s.Key[0] >= 'A' && s.Key[0] <= 'Z' || s.Key[0] >= '0' && s.Key[0] <= '9')
}

func Defaults() Settings {
	return Settings{Settings: breaks.Defaults()}
}

func (s Settings) Validate() error {
	if !s.Language.Valid() {
		return fmt.Errorf("unsupported language %q", s.Language)
	}
	switch s.Theme {
	case ThemeDark, ThemeSystem, ThemeLight:
	default:
		return fmt.Errorf("unsupported theme %q", s.Theme)
	}
	if !s.PreviewShortcut.Valid() {
		return errors.New("invalid preview shortcut")
	}
	return s.Settings.Validate()
}

// Store whole seconds as int32 so even invalid inputs convert without overflowing time.Duration.
type fileSettings struct {
	Interval        int32         `json:"interval"`
	Short           fileRule      `json:"short"`
	Long            fileRule      `json:"long"`
	SetupConfirmed  bool          `json:"setupConfirmed"`
	Language        i18n.Language `json:"language,omitempty"`
	Theme           Theme         `json:"theme,omitempty"`
	PreviewShortcut Shortcut      `json:"previewShortcut,omitzero"`
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
		Interval:        int32(s.Interval / time.Second),
		Short:           rule(s.Short),
		Long:            rule(s.Long),
		SetupConfirmed:  true,
		Language:        s.Language,
		Theme:           s.Theme,
		PreviewShortcut: s.PreviewShortcut,
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
		Settings:        breaks.Settings{Interval: time.Duration(f.Interval) * time.Second, Short: rule(f.Short), Long: rule(f.Long)},
		Language:        f.Language,
		Theme:           f.Theme,
		PreviewShortcut: f.PreviewShortcut,
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

// DurabilityError reports a replacement that succeeded but could not be made
// durable because its parent directory could not be synced.
type DurabilityError struct{ Err error }

func (e *DurabilityError) Error() string {
	return "settings replaced but directory sync failed: " + e.Err.Error()
}
func (e *DurabilityError) Unwrap() error { return e.Err }

// Save validates and writes settings, recording that setup has been confirmed.
// A DurabilityError means the new settings are already visible at path.
func Save(path string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(encode(settings), "", "  ")
	if err != nil {
		return err
	}
	return replaceSettingsFile(path, append(data, '\n'), syncSettingsDir)
}

func replaceSettingsFile(path string, data []byte, syncDir func(string) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		return &DurabilityError{Err: err}
	}
	return nil
}
