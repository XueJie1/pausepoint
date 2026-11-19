// Package i18n contains Pausepoint's user interface translations.
package i18n

import "fmt"

// Language is a supported language code, or System to use the system language.
type Language string

const (
	System  Language = ""
	Arabic  Language = "ar"
	Chinese Language = "zh"
	English Language = "en"
	French  Language = "fr"
	Russian Language = "ru"
	Spanish Language = "es"
)

// Choice pairs a language code with the language's name in its own script.
type Choice struct {
	Code Language
	Name string
}

func Languages() []Choice {
	return []Choice{
		{Arabic, "العربية"},
		{Chinese, "简体中文"},
		{English, "English"},
		{French, "Français"},
		{Russian, "Русский"},
		{Spanish, "Español"},
	}
}

func (l Language) Valid() bool {
	return l == System || catalogs[l] != nil
}

func (l Language) RightToLeft() bool {
	return l == Arabic
}

// Key identifies a message shared by all six catalogs.
type Key int

const (
	LanguageLabel Key = iota
	SystemLanguage
	BreakWindowTitle
	KeepRunningReason
	PauseReminders
	ResumeReminders
	StartBreakNow
	TraySettings
	Quit
	BreakReminders
	Schedule
	WorkInterval
	ScheduleHelp
	ShortBreak
	LongBreak
	RuleHelp
	SaveSettings
	ConfirmSettings
	RestoreDefaults
	DefaultsPending
	Close
	Duration
	AdvanceNotice
	PostponeBy
	ShortDuration
	ShortNotice
	ShortPostpone
	LongDuration
	LongNotice
	LongPostpone
	ShortStrict
	LongStrict
	SecondsLabel
	ShortIdea
	LongIdea
	StrictOn
	Postpone
	SkipBreak
	Paused
	Sleeping
	ActiveStatus
	WaitingStatus
	LoadFailed
	ConfigUnavailable
	SaveFailed
	Saved
	NotificationsUnavailable
	WholeNumberRange
	PositiveSeconds
	NoticeBelowInterval
	NonNegativeSeconds
	keyCount
)

type catalog [keyCount]string

var catalogs = map[Language]*catalog{
	Arabic: &arabic, Chinese: &chinese, English: &english,
	French: &french, Russian: &russian, Spanish: &spanish,
}

// Text renders a message. English is the fallback for unresolved languages and missing translations.
func (l Language) Text(key Key, args ...any) string {
	if key < 0 || key >= keyCount {
		return ""
	}
	text := english[key]
	if c := catalogs[l]; c != nil && c[key] != "" {
		text = c[key]
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}
