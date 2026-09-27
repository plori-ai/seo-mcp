package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BacklinksProfileRequest selects a target and one page of backlink rows.
// Nil options use page 1, pageSize 100, and hideSpam true. The default sort is
// firstSeen descending and the default mode is one_per_domain.
type BacklinksProfileRequest struct {
	Target    string                  `json:"target"`
	Scope     string                  `json:"scope,omitempty"`
	Page      *int                    `json:"page,omitempty"`
	PageSize  *int                    `json:"pageSize,omitempty"`
	SortField string                  `json:"sortField,omitempty"`
	SortOrder string                  `json:"sortOrder,omitempty"`
	Filters   BacklinksProfileFilters `json:"filters,omitempty"`
	Mode      string                  `json:"mode,omitempty"`
	HideSpam  *bool                   `json:"hideSpam,omitempty"`
}

// BacklinksProfileFilters matches source URLs, authority/spam ranges, and
// link status. Include/exclude split on commas or plus signs. Numeric bounds
// accept JSON numbers or numeric strings; blank strings leave a bound unset.
type BacklinksProfileFilters struct {
	Include          string   `json:"include,omitempty"`
	Exclude          string   `json:"exclude,omitempty"`
	MinDomainRank    *float64 `json:"minDomainRank,omitempty"`
	MaxDomainRank    *float64 `json:"maxDomainRank,omitempty"`
	MinLinkAuthority *float64 `json:"minLinkAuthority,omitempty"`
	MaxLinkAuthority *float64 `json:"maxLinkAuthority,omitempty"`
	MinSpamScore     *float64 `json:"minSpamScore,omitempty"`
	MaxSpamScore     *float64 `json:"maxSpamScore,omitempty"`
	LinkType         string   `json:"linkType,omitempty"`
	HideLost         bool     `json:"hideLost,omitempty"`
	HideBroken       bool     `json:"hideBroken,omitempty"`
	DomainFrom       string   `json:"domainFrom,omitempty"`
}

// UnmarshalJSON applies the reference's numeric-string filter coercion.
func (f *BacklinksProfileFilters) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, key := range []string{"minDomainRank", "maxDomainRank", "minLinkAuthority", "maxLinkAuthority", "minSpamScore", "maxSpamScore"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		number, err := backlinksFilterNumber(value)
		if err != nil {
			return inputErrorf("filters.%s must be a finite number or numeric string", key)
		}
		if number == nil {
			delete(fields, key)
		} else {
			fields[key], err = json.Marshal(*number)
			if err != nil {
				return err
			}
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain BacklinksProfileFilters
	var decoded plain
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		return err
	}
	*f = BacklinksProfileFilters(decoded)
	return nil
}

var backlinksDecimalNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func backlinksFilterNumber(raw json.RawMessage) (*float64, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil
		}
		if len(value) > 2 && value[0] == '0' && strings.ContainsAny(value[1:2], "xXoObB") {
			// Number(string) also accepts unsigned hexadecimal, octal and binary.
			base := 16
			switch value[1] {
			case 'o', 'O':
				base = 8
			case 'b', 'B':
				base = 2
			}
			integer, ok := new(big.Int).SetString(value[2:], base)
			if !ok || strings.ContainsAny(value[2:], "+-_") {
				return nil, fmt.Errorf("invalid number")
			}
			number, _ = new(big.Float).SetInt(integer).Float64()
		} else {
			if !backlinksDecimalNumber.MatchString(value) {
				return nil, fmt.Errorf("invalid number")
			}
			var err error
			number, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("invalid number")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, fmt.Errorf("invalid number")
	}
	return &number, nil
}

// BacklinksProfileResult is the detailed backlink page and its research scope.
type BacklinksProfileResult struct {
	Target    string               `json:"target"`
	Scope     string               `json:"scope"`
	Backlinks BacklinksProfilePage `json:"backlinks"`
}

// BacklinksProfilePage contains the requested rows and pagination metadata.
type BacklinksProfilePage struct {
	Rows       []BacklinkRow `json:"rows"`
	TotalCount *float64      `json:"totalCount"`
	HasMore    bool          `json:"hasMore"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
	FetchedAt  string        `json:"fetchedAt"`
}

// BacklinkRow contains nullable link metrics, attributes, and status flags.
type BacklinkRow struct {
	DomainFrom     *string  `json:"domainFrom"`
	URLFrom        *string  `json:"urlFrom"`
	URLTo          *string  `json:"urlTo"`
	Anchor         *string  `json:"anchor"`
	ItemType       *string  `json:"itemType"`
	IsDofollow     *bool    `json:"isDofollow"`
	RelAttributes  []string `json:"relAttributes"`
	Rank           *float64 `json:"rank"`
	DomainFromRank *float64 `json:"domainFromRank"`
	PageFromRank   *float64 `json:"pageFromRank"`
	SpamScore      *float64 `json:"spamScore"`
	FirstSeen      *string  `json:"firstSeen"`
	LastSeen       *string  `json:"lastSeen"`
	IsLost         bool     `json:"isLost"`
	IsBroken       bool     `json:"isBroken"`
	LinksCount     *float64 `json:"linksCount"`
}

type backlinksProfileItem struct {
	DomainFrom      *string  `json:"domain_from"`
	URLFrom         *string  `json:"url_from"`
	URLTo           *string  `json:"url_to"`
	Anchor          *string  `json:"anchor"`
	ItemType        *string  `json:"item_type"`
	Dofollow        *bool    `json:"dofollow"`
	RelAttributes   []string `json:"rel_attributes"`
	Attributes      []string `json:"attributes"`
	Rank            *float64 `json:"rank"`
	DomainFromRank  *float64 `json:"domain_from_rank"`
	PageFromRank    *float64 `json:"page_from_rank"`
	SpamScore       *float64 `json:"backlink_spam_score"`
	LegacySpamScore *float64 `json:"backlinks_spam_score"`
	FirstSeen       *string  `json:"first_seen"`
	LastVisited     *string  `json:"last_visited"`
	LostDate        *string  `json:"lost_date"`
	IsLost          *bool    `json:"is_lost"`
	IsBroken        *bool    `json:"is_broken"`
	LinksCount      *float64 `json:"links_count"`
}

// BacklinksProfile returns one page of backlinks for a domain, subfolder, or
// exact URL. Each call is billed by DataForSEO to the operator's account and
// requires Backlinks API access.
func (c *Client) BacklinksProfile(ctx context.Context, req BacklinksProfileRequest) (*BacklinksProfileResult, error) {
	if err := validateBacklinksProfileRequest(req); err != nil {
		return nil, err
	}
	target, err := normalizeBacklinksTarget(req.Target, req.Scope)
	if err != nil {
		return nil, err
	}
	page, pageSize := 1, 100
	if req.Page != nil {
		page = *req.Page
	}
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}
	if page-1 > int(^uint(0)>>1)/pageSize {
		return nil, inputErrorf("page is too large to calculate the result offset")
	}
	offset := (page - 1) * pageSize
	filters, err := backlinksProfileClauses(req.Filters, target, req.HideSpam == nil || *req.HideSpam)
	if err != nil {
		return nil, err
	}
	field := "first_seen"
	switch req.SortField {
	case "rank":
		field = "rank"
	case "domainRank":
		field = "domain_from_rank"
	case "spamScore":
		field = "backlink_spam_score"
	}
	order := req.SortOrder
	if order == "" {
		order = "desc"
	}
	mode := req.Mode
	if mode == "" {
		mode = "one_per_domain"
	}
	payload := backlinksCommonPayload(target)
	payload["limit"], payload["offset"], payload["order_by"], payload["mode"] = pageSize, offset, []string{field + "," + order}, mode
	if len(filters) > 0 {
		payload["filters"] = filters
	}
	var response struct {
		Items      []*backlinksProfileItem `json:"items"`
		TotalCount json.RawMessage         `json:"total_count"`
	}
	if err := c.readBacklinksResult(ctx, "backlinks", payload, &response); err != nil {
		return nil, err
	}
	rows := make([]BacklinkRow, 0, len(response.Items))
	for _, item := range response.Items {
		if item == nil {
			return nil, fmt.Errorf("backlinks profile: invalid null row")
		}
		attributes := item.RelAttributes
		if attributes == nil {
			attributes = item.Attributes
		}
		if attributes == nil {
			attributes = []string{}
		}
		lastSeen := item.LostDate
		if lastSeen == nil {
			lastSeen = item.LastVisited
		}
		isLost := item.LostDate != nil && *item.LostDate != ""
		if item.IsLost != nil {
			isLost = *item.IsLost
		}
		rows = append(rows, BacklinkRow{
			DomainFrom: item.DomainFrom, URLFrom: item.URLFrom, URLTo: item.URLTo,
			Anchor: item.Anchor, ItemType: item.ItemType, IsDofollow: item.Dofollow,
			RelAttributes: attributes, Rank: item.Rank, DomainFromRank: item.DomainFromRank,
			PageFromRank: item.PageFromRank, SpamScore: firstBacklinksNumber(item.SpamScore, item.LegacySpamScore),
			FirstSeen: item.FirstSeen, LastSeen: lastSeen, IsLost: isLost,
			IsBroken: valueOrZero(item.IsBroken), LinksCount: item.LinksCount,
		})
	}
	total := backlinksTotalCount(response.TotalCount)
	hasMore := len(rows) == pageSize
	if total != nil {
		hasMore = float64(offset)+float64(len(rows)) < *total
	}
	return &BacklinksProfileResult{Target: target.display, Scope: target.scope, Backlinks: BacklinksProfilePage{
		Rows: rows, TotalCount: total, HasMore: hasMore, Page: page, PageSize: pageSize, FetchedAt: researchTimestamp(time.Now()),
	}}, nil
}

func validateBacklinksProfileRequest(req BacklinksProfileRequest) error {
	if n := researchStringLength(req.Target); n < 1 || n > 2048 {
		return inputErrorf("target must contain 1 to 2048 characters")
	}
	if req.Page != nil && *req.Page < 1 {
		return inputErrorf("page must be a positive integer")
	}
	if req.PageSize != nil && *req.PageSize != 50 && *req.PageSize != 100 && *req.PageSize != 200 {
		return inputErrorf("pageSize must be 50, 100, or 200")
	}
	switch req.SortField {
	case "", "rank", "domainRank", "spamScore", "firstSeen":
	default:
		return inputErrorf("Invalid sortField: %s", req.SortField)
	}
	switch req.SortOrder {
	case "", "asc", "desc":
	default:
		return inputErrorf("Invalid sortOrder: %s", req.SortOrder)
	}
	switch req.Mode {
	case "", "one_per_domain", "as_is":
	default:
		return inputErrorf("Invalid mode: %s", req.Mode)
	}
	switch req.Filters.LinkType {
	case "", "dofollow", "nofollow":
	default:
		return inputErrorf("Invalid filters.linkType: %s", req.Filters.LinkType)
	}
	if researchStringLength(req.Filters.DomainFrom) > 255 {
		return inputErrorf("filters.domainFrom must contain at most 255 characters")
	}
	return nil
}

func backlinksProfileClauses(filters BacklinksProfileFilters, target backlinksTarget, hideSpam bool) ([]any, error) {
	var clauses []any
	count := 0
	if target.scope == "subfolder" {
		clauses = backlinksScopeClauses(target)
		count = 4
	}
	var include []any
	for _, term := range backlinksProfileTerms(filters.Include) {
		include = append(include, []any{"url_from", "ilike", "%" + escapeResearchLike(term) + "%"})
	}
	count += len(include)
	if len(include) == 1 {
		clauses = append(clauses, include[0])
	} else if len(include) > 1 {
		clauses = append(clauses, joinResearchClauses(include, "or"))
	}
	for _, term := range backlinksProfileTerms(filters.Exclude) {
		clauses = append(clauses, []any{"url_from", "not_ilike", "%" + escapeResearchLike(term) + "%"})
		count++
	}
	for _, bound := range []struct {
		field, operator string
		value           *float64
	}{
		{"domain_from_rank", ">=", filters.MinDomainRank}, {"domain_from_rank", "<=", filters.MaxDomainRank},
		{"rank", ">=", filters.MinLinkAuthority}, {"rank", "<=", filters.MaxLinkAuthority},
		{"backlink_spam_score", ">=", filters.MinSpamScore}, {"backlink_spam_score", "<=", filters.MaxSpamScore},
	} {
		if bound.value != nil {
			if math.IsNaN(*bound.value) || math.IsInf(*bound.value, 0) {
				return nil, inputErrorf("Numeric filter bounds must be finite")
			}
			clauses = append(clauses, []any{bound.field, bound.operator, *bound.value})
			count++
		}
	}
	if filters.LinkType != "" {
		clauses = append(clauses, []any{"dofollow", "=", filters.LinkType == "dofollow"})
		count++
	}
	if filters.HideLost {
		clauses = append(clauses, []any{"is_lost", "=", false})
		count++
	}
	if filters.HideBroken {
		clauses = append(clauses, []any{"is_broken", "=", false})
		count++
	}
	if filters.DomainFrom != "" {
		clauses = append(clauses, []any{"domain_from", "=", filters.DomainFrom})
		count++
	}
	if hideSpam {
		clauses = append(clauses, []any{"backlink_spam_score", "<=", 40})
		count++
	}
	if count > 8 {
		return nil, inputErrorf("Too many filter conditions (%d of 8 max).", count)
	}
	return joinResearchClauses(clauses, "and"), nil
}

func backlinksProfileTerms(value string) []string {
	var terms []string
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == ',' || r == '+' }) {
		if term = strings.TrimSpace(term); term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}
