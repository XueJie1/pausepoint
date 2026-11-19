package i18n

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCatalogs(t *testing.T) {
	choices := Languages()
	if len(choices) != 6 {
		t.Fatalf("got %d language choices, want 6", len(choices))
	}
	seen := make(map[Language]bool)
	for _, choice := range choices {
		t.Run(string(choice.Code), func(t *testing.T) {
			if choice.Code == System || !choice.Code.Valid() || seen[choice.Code] || choice.Name == "" {
				t.Fatalf("invalid or duplicate choice: %#v", choice)
			}
			seen[choice.Code] = true
			for key, text := range catalogs[choice.Code] {
				if strings.TrimSpace(text) == "" || !utf8.ValidString(text) {
					t.Errorf("missing or invalid translation for key %d (%q)", key, english[key])
				}
				var args []any
				switch Key(key) {
				case ActiveStatus, WaitingStatus:
					args = []any{"break-kind", "08:23"}
				case ScheduleSummary:
					args = []any{"10 min", "20 s", "5 min"}
				case SecondsLabel, WholeNumberRange, PositiveSeconds, NoticeBelowInterval, NonNegativeSeconds:
					args = []any{"field-name"}
				}
				formatted := choice.Code.Text(Key(key), args...)
				if strings.Contains(formatted, "%!") {
					t.Errorf("unexpected format for key %d (%q): %q", key, english[key], formatted)
				}
				for _, arg := range args {
					if !strings.Contains(formatted, arg.(string)) {
						t.Errorf("key %d (%q) omitted argument %q", key, english[key], arg)
					}
				}
			}
		})
	}
}

func TestFallback(t *testing.T) {
	if !System.Valid() || Language("de").Valid() {
		t.Fatal("expected the system preference to be valid and unsupported codes to be invalid")
	}
	for _, lang := range []Language{System, "de"} {
		if got := lang.Text(BreakReminders); got != "Break reminders" {
			t.Errorf("%q fallback produced %q, want English", lang, got)
		}
	}
}
