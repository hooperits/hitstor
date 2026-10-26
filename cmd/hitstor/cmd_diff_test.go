package main

import (
	"testing"

	"github.com/hooperits/hitstor/internal/fs"
)

func TestContentEqual(t *testing.T) {
	id1 := fs.ID{1, 2, 3}
	id2 := fs.ID{4, 5, 6}
	id3 := fs.ID{1, 2, 3}

	tests := []struct {
		name string
		a, b []fs.ID
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []fs.ID{}, []fs.ID{}, true},
		{"equal single", []fs.ID{id1}, []fs.ID{id3}, true},
		{"different single", []fs.ID{id1}, []fs.ID{id2}, false},
		{"different length", []fs.ID{id1}, []fs.ID{id1, id2}, false},
		{"equal multi", []fs.ID{id1, id2}, []fs.ID{id3, id2}, true},
		{"nil vs empty", nil, []fs.ID{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("contentEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
