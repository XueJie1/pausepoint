package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pausepoint/internal/breaks"
)

func TestLoadDefaultsAndPartialSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := breaks.Defaults()
	if got, confirmed, err := Load(path); err != nil || got != want || confirmed {
		t.Fatalf("missing file: got %+v, confirmed %t, %v", got, confirmed, err)
	}
	data := []byte(`{"interval":120,"short":{"strict":true,"notice":0},"futureKey":5}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	want.Interval = 2 * time.Minute
	want.Short.Strict, want.Short.Notice = true, 0
	if got, confirmed, err := Load(path); err != nil || got != want || !confirmed {
		t.Fatalf("legacy partial file: got %+v, confirmed %t, %v, want %+v", got, confirmed, err, want)
	}
}

func TestLoadUnconfirmedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"setupConfirmed":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got, confirmed, err := Load(path); err != nil || got != breaks.Defaults() || confirmed {
		t.Fatalf("unconfirmed settings: got %+v, confirmed %t, %v", got, confirmed, err)
	}
}

func TestLoadInvalidSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, data := range []string{`{`, `null`, `{"interval":0}`, `{"short":{"notice":600}}`, `{"long":{"duration":1.5}}`, `{"interval":9223372036854775807}`, `{"setupConfirmed":"true"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got, confirmed, err := Load(path); err == nil || got != breaks.Defaults() || confirmed {
			t.Fatalf("loading %s: got %+v, confirmed %t, %v", data, got, confirmed, err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Fatalf("changed invalid file: got %s, %v", got, err)
		}
	}
}

func TestSaveReplacesOnlyWithValidSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pausepoint", "settings.json")
	for _, interval := range []time.Duration{10 * time.Minute, 9 * time.Minute} {
		want := breaks.Defaults()
		want.Interval, want.Long.Strict, want.Short.Postpone = interval, true, 0
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		if err := Save(path, breaks.Settings{}); err == nil {
			t.Fatal("saved invalid settings")
		}
		if got, confirmed, err := Load(path); err != nil || got != want || !confirmed {
			t.Fatalf("load after save: got %+v, confirmed %t, %v, want %+v", got, confirmed, err, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var saved fileSettings
		if err := json.Unmarshal(data, &saved); err != nil || !saved.SetupConfirmed {
			t.Fatalf("saved confirmation: got %t, %v", saved.SetupConfirmed, err)
		}
	}
}
