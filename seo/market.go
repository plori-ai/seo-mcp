package seo

import (
	"slices"
	"strings"
)

// locationByCode indexes locations by DataForSEO location code.
var locationByCode = func() map[int]*location {
	m := make(map[int]*location, len(locations))
	for i := range locations {
		m[locations[i].code] = &locations[i]
	}
	return m
}()

// servedLanguages returns the languages DataForSEO Labs serves for loc.
func (loc *location) servedLanguages() []string {
	if loc.languages != nil {
		return loc.languages
	}
	return []string{loc.language}
}

// adsOnlyLocation reports whether keyword data for code comes from the Google
// Ads endpoints because DataForSEO Labs does not cover the country. Labs-only
// operations (ranked keywords, domain overview) must reject such a location.
func adsOnlyLocation(code int) bool {
	loc := locationByCode[code]
	return loc != nil && loc.adsOnly
}

// resolveMarket applies OpenSEO's resolveMarket and assertLanguageForLocation rules:
// a zero locationCode means the client's default market; an empty languageCode means the
// default language when the location is the default location, otherwise the location's
// primary language; an unsupported location or a language not served for the location is an *InputError.
//
// The language is resolved together with the location because the default
// language was chosen for the default location and may not be served for
// another one. Validating here keeps a request DataForSEO would reject, and
// still charge for, from being sent.
func (c *Client) resolveMarket(locationCode int, languageCode string) (Market, error) {
	if locationCode == 0 {
		locationCode = c.market.LocationCode
	}
	loc := locationByCode[locationCode]
	if loc == nil {
		return Market{}, inputErrorf("location code %d is not supported. Use a DataForSEO country location code, for example 2840 (United States), 2826 (United Kingdom) or 2276 (Germany).", locationCode)
	}
	if languageCode == "" {
		if locationCode == c.market.LocationCode {
			languageCode = c.market.LanguageCode
		} else {
			languageCode = loc.language
		}
	}
	if !supportedLanguages[languageCode] {
		return Market{}, inputErrorf("language code %q is not supported. Use a DataForSEO language code such as 'en', 'es', 'de' or 'fr'.", languageCode)
	}
	// DataForSEO publishes per-country language lists only for Labs
	// countries; for Google Ads countries any supported language is sent.
	if !loc.adsOnly && !slices.Contains(loc.servedLanguages(), languageCode) {
		return Market{}, inputErrorf("language %q is not available for %s (%d). Available: %s.",
			languageCode, loc.name, loc.code, strings.Join(loc.servedLanguages(), ", "))
	}
	return Market{LocationCode: locationCode, LanguageCode: languageCode}, nil
}

// Validate reports whether DataForSEO serves m: a supported country location
// code and a language served there. Both fields are required. Use it to check
// a market before passing it to WithDefaultMarket; the error is an *InputError.
func (m Market) Validate() error {
	if m.LocationCode == 0 || m.LanguageCode == "" {
		return inputErrorf("a market needs both a location code and a language code")
	}
	_, err := (&Client{market: m}).resolveMarket(m.LocationCode, m.LanguageCode)
	return err
}
