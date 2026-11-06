package main

import "testing"

func TestParseSizeSuffixes(t *testing.T) {
	cases := map[string]uint64{
		"500":       500,
		"10B":       10,
		"2KiB":      2048,
		"500MiB":    500 << 20,
		"1GiB":      1 << 30,
		"1TiB":      1 << 40,
		"3KB":       3000,
		"5MB":       5_000_000,
		"2GB":       2_000_000_000,
		"1TB":       1_000_000_000_000,
		" 1.5 GiB ": 3 << 29,
	}
	// Map iteration made suffix matching random; repeat to catch it.
	for i := 0; i < 50; i++ {
		for in, want := range cases {
			got, err := parseSize(in)
			if err != nil || got != want {
				t.Fatalf("parseSize(%q) = %d, %v; want %d", in, got, err, want)
			}
		}
	}
}
