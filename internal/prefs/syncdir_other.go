//go:build !unix

package prefs

func syncSettingsDir(string) error { return nil }
