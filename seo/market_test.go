package seo

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestResolveMarket(t *testing.T) {
	us := New(nil)
	canadaFR := New(nil, WithDefaultMarket(Market{LocationCode: 2124, LanguageCode: "fr"}))
	tests := []struct {
		name     string
		client   *Client
		location int
		language string
		want     Market
		wantErr  string
	}{
		{"default market", us, 0, "", Market{2840, "en"}, ""},
		{"default location, other served language", us, 0, "es", Market{2840, "es"}, ""},
		{"other location takes its own language", us, 2276, "", Market{2276, "de"}, ""},
		{"explicit pair", us, 2756, "it", Market{2756, "it"}, ""},
		{"client default market", canadaFR, 0, "", Market{2124, "fr"}, ""},
		{"client default location named explicitly", canadaFR, 2124, "", Market{2124, "fr"}, ""},
		{"client default location, other language", canadaFR, 0, "en", Market{2124, "en"}, ""},
		{"location off the client default", canadaFR, 2840, "", Market{2840, "en"}, ""},
		{"Google Ads country takes any supported language", us, 2352, "en", Market{2352, "en"}, ""},
		{"Google Ads country default language", us, 2352, "", Market{2352, "is"}, ""},
		{"language not served for a Labs country", us, 2840, "ru", Market{}, `language "ru" is not available for United States (2840). Available: en, es.`},
		{"default location with an unserved language", canadaFR, 0, "de", Market{}, "Available: en, fr."},
		{"unknown location", us, 9999, "", Market{}, "location code 9999 is not supported"},
		{"negative location", us, -1, "en", Market{}, "location code -1 is not supported"},
		{"unsupported language", us, 2352, "xx", Market{}, `language code "xx" is not supported`},
		{"region subtag", us, 2840, "en-US", Market{}, `language code "en-US" is not supported`},
		{"unknown client default", New(nil, WithDefaultMarket(Market{LocationCode: 1, LanguageCode: "en"})), 0, "", Market{}, "location code 1 is not supported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.client.resolveMarket(tc.location, tc.language)
			if tc.wantErr != "" {
				var ie *InputError
				if !errors.As(err, &ie) || !strings.Contains(ie.Msg, tc.wantErr) {
					t.Fatalf("err = %v, want *InputError containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAdsOnlyLocation(t *testing.T) {
	for code, want := range map[int]bool{2352: true, 2020: true, 2840: false, 2276: false, 9999: false} {
		if got := adsOnlyLocation(code); got != want {
			t.Errorf("adsOnlyLocation(%d) = %v, want %v", code, got, want)
		}
	}
}

// TestLocationTable checks the invariants resolveMarket relies on.
func TestLocationTable(t *testing.T) {
	seen := make(map[int]bool)
	for _, loc := range locations {
		if seen[loc.code] {
			t.Errorf("location %d listed twice", loc.code)
		}
		seen[loc.code] = true
		if loc.name == "" {
			t.Errorf("location %d has no name", loc.code)
		}
		if !supportedLanguages[loc.language] {
			t.Errorf("%s (%d): default language %q is not a supported language", loc.name, loc.code, loc.language)
		}
		if loc.languages != nil && !slices.Contains(loc.languages, loc.language) {
			t.Errorf("%s (%d): languages %v do not include the default %q", loc.name, loc.code, loc.languages, loc.language)
		}
		for _, lang := range loc.languages {
			if !supportedLanguages[lang] {
				t.Errorf("%s (%d): language %q is not a supported language", loc.name, loc.code, lang)
			}
		}
		if loc.adsOnly && loc.languages != nil {
			t.Errorf("%s (%d): a Google Ads country has no per-country language list", loc.name, loc.code)
		}
	}
	if !seen[DefaultLocationCode] {
		t.Errorf("default location %d is not in the table", DefaultLocationCode)
	}
}
