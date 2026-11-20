package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestEmbeddedIconSizes(t *testing.T) {
	for _, tt := range []struct {
		name string
		png  []byte
		size int
	}{
		{"window", windowIcon, 256},
		{"tray", trayIcon, 32},
	} {
		t.Run(tt.name, func(t *testing.T) {
			icon, err := png.Decode(bytes.NewReader(tt.png))
			if err != nil {
				t.Fatal(err)
			}
			if b := icon.Bounds(); b.Dx() != tt.size || b.Dy() != tt.size {
				t.Errorf("icon size = %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.size, tt.size)
			}
		})
	}
}
