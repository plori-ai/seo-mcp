package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// BacklinksOverviewRequest selects a domain, subfolder, or page. HideSpam
// defaults to true and controls the referring-domain list only.
type BacklinksOverviewRequest struct {
	Target   string `json:"target"`
	Scope    string `json:"scope,omitempty"`
	HideSpam *bool  `json:"hideSpam,omitempty"`
}

// BacklinksOverviewResult preserves the reference tool's overview wrapper.
// ReferringDomains is absent for subfolders; ScopeNote is present only when
// the scope has a provider limitation to explain.
type BacklinksOverviewResult struct {
	Target           string                    `json:"target"`
	Scope            string                    `json:"scope"`
	ScopeNote        string                    `json:"scopeNote,omitempty"`
	Overview         BacklinksOverviewEnvelope `json:"overview"`
	ReferringDomains *ReferringDomainsPage     `json:"referringDomains,omitempty"`
}

// BacklinksOverviewEnvelope contains the profile returned by the service.
type BacklinksOverviewEnvelope struct {
	Overview BacklinksProfile `json:"overview"`
}

// BacklinksProfile contains summary metrics and historical trend series.
type BacklinksProfile struct {
	Target        string                  `json:"target"`
	DisplayTarget string                  `json:"displayTarget"`
	Scope         string                  `json:"scope"`
	Summary       BacklinksSummary        `json:"summary"`
	Trends        []BacklinksTrend        `json:"trends"`
	NewLostTrends []BacklinksNewLostTrend `json:"newLostTrends"`
	FetchedAt     string                  `json:"fetchedAt"`
}

// BacklinksSummary contains nullable provider metrics for the target.
type BacklinksSummary struct {
	Rank                 *float64 `json:"rank"`
	Backlinks            *float64 `json:"backlinks"`
	ReferringPages       *float64 `json:"referringPages"`
	ReferringDomains     *float64 `json:"referringDomains"`
	BrokenBacklinks      *float64 `json:"brokenBacklinks"`
	BrokenPages          *float64 `json:"brokenPages"`
	BacklinksSpamScore   *float64 `json:"backlinksSpamScore"`
	TargetSpamScore      *float64 `json:"targetSpamScore"`
	NewBacklinks         *float64 `json:"newBacklinks"`
	LostBacklinks        *float64 `json:"lostBacklinks"`
	NewReferringDomains  *float64 `json:"newReferringDomains"`
	LostReferringDomains *float64 `json:"lostReferringDomains"`
}

// BacklinksTrend is one dated set of backlink, domain, and rank metrics.
type BacklinksTrend struct {
	Date             string   `json:"date"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referringDomains"`
	Rank             *float64 `json:"rank"`
}

// BacklinksNewLostTrend is one dated set of new and lost link metrics.
type BacklinksNewLostTrend struct {
	Date                 string   `json:"date"`
	NewBacklinks         *float64 `json:"newBacklinks"`
	LostBacklinks        *float64 `json:"lostBacklinks"`
	NewReferringDomains  *float64 `json:"newReferringDomains"`
	LostReferringDomains *float64 `json:"lostReferringDomains"`
}

// ReferringDomain is a referring-domain row with explicit nulls for missing metrics.
type ReferringDomain struct {
	Domain          *string  `json:"domain"`
	Backlinks       *float64 `json:"backlinks"`
	ReferringPages  *float64 `json:"referringPages"`
	Rank            *float64 `json:"rank"`
	SpamScore       *float64 `json:"spamScore"`
	FirstSeen       *string  `json:"firstSeen"`
	BrokenBacklinks *float64 `json:"brokenBacklinks"`
	BrokenPages     *float64 `json:"brokenPages"`
}

// ReferringDomainsPage contains the first 100 referring domains by backlink count.
type ReferringDomainsPage struct {
	Rows       []ReferringDomain `json:"rows"`
	TotalCount *float64          `json:"totalCount"`
	HasMore    bool              `json:"hasMore"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	FetchedAt  string            `json:"fetchedAt"`
}

type backlinksAPIMetrics struct {
	Rank                 *float64 `json:"rank"`
	Backlinks            *float64 `json:"backlinks"`
	ReferringPages       *float64 `json:"referring_pages"`
	ReferringDomains     *float64 `json:"referring_domains"`
	BrokenBacklinks      *float64 `json:"broken_backlinks"`
	BrokenPages          *float64 `json:"broken_pages"`
	SpamScore            *float64 `json:"backlinks_spam_score"`
	NewBacklinks         *float64 `json:"new_backlinks"`
	LostBacklinks        *float64 `json:"lost_backlinks"`
	NewReferringDomains  *float64 `json:"new_referring_domains"`
	LostReferringDomains *float64 `json:"lost_referring_domains"`
	NewRefferingDomains  *float64 `json:"new_reffering_domains"`
	LostRefferingDomains *float64 `json:"lost_reffering_domains"`
	Info                 *struct {
		TargetSpamScore *float64 `json:"target_spam_score"`
	} `json:"info"`
}

type backlinksHistoryItem struct {
	backlinksAPIMetrics
	Date *string `json:"date"`
}

type referringDomainItem struct {
	Domain          *string  `json:"domain"`
	Backlinks       *float64 `json:"backlinks"`
	ReferringPages  *float64 `json:"referring_pages"`
	Rank            *float64 `json:"rank"`
	SpamScore       *float64 `json:"backlinks_spam_score"`
	FirstSeen       *string  `json:"first_seen"`
	BrokenBacklinks *float64 `json:"broken_backlinks"`
	BrokenPages     *float64 `json:"broken_pages"`
}

// BacklinksOverview returns a summary, trends, and up to 100 referring domains.
// Subfolders have filtered counts only. Each call is billed by DataForSEO and
// requires the account's Backlinks API access.
func (c *Client) BacklinksOverview(ctx context.Context, req BacklinksOverviewRequest) (*BacklinksOverviewResult, error) {
	if req.Target == "" {
		return nil, inputErrorf("target must contain at least 1 character")
	}
	target, err := normalizeBacklinksTarget(req.Target, req.Scope)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	profile := BacklinksProfile{Target: target.apiTarget, DisplayTarget: target.display, Scope: target.scope, Trends: []BacklinksTrend{}, NewLostTrends: []BacklinksNewLostTrend{}, FetchedAt: researchTimestamp(now)}
	result := &BacklinksOverviewResult{Target: target.display, Scope: target.scope}
	if target.scope == "subfolder" {
		result.ScopeNote = "Counts are computed from filtered backlink totals; rank, trends, and the referring-domains breakdown aren't available for subfolders."
		all, err := c.backlinksSubfolderCount(ctx, target, "as_is")
		if err != nil {
			return nil, err
		}
		perDomain, err := c.backlinksSubfolderCount(ctx, target, "one_per_domain")
		if err != nil {
			return nil, err
		}
		profile.Summary.Backlinks, profile.Summary.ReferringDomains = all, perDomain
		result.Overview.Overview = profile
		return result, nil
	}
	if target.scope == "domain" {
		result.ScopeNote = "Summary excludes subdomains; trend data includes subdomains (provider limitation)."
	}
	var summary backlinksAPIMetrics
	var history struct {
		Items []backlinksHistoryItem `json:"items"`
	}
	var refs struct {
		Items      []referringDomainItem `json:"items"`
		TotalCount json.RawMessage       `json:"total_count"`
	}
	refPayload := backlinksCommonPayload(target)
	refPayload["limit"], refPayload["offset"], refPayload["order_by"] = 100, 0, []string{"backlinks,desc"}
	if req.HideSpam == nil || *req.HideSpam {
		refPayload["filters"] = []any{[]any{"backlinks_spam_score", "<=", 40}}
	}
	jobs := []func() error{
		func() error { return c.readBacklinksResult(ctx, "summary", backlinksCommonPayload(target), &summary) },
		func() error { return c.readBacklinksResult(ctx, "referring_domains", refPayload, &refs) },
	}
	if target.scope != "exact_url" {
		from, to := backlinksDateRange(now)
		jobs = append(jobs, func() error {
			return c.readBacklinksResult(ctx, "history", map[string]any{"target": target.apiTarget, "date_from": from, "date_to": to, "rank_scale": "one_hundred"}, &history)
		})
	}
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = job() }()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	profile.Summary = BacklinksSummary{
		Rank: summary.Rank, Backlinks: summary.Backlinks, ReferringPages: summary.ReferringPages, ReferringDomains: summary.ReferringDomains,
		BrokenBacklinks: summary.BrokenBacklinks, BrokenPages: summary.BrokenPages, BacklinksSpamScore: summary.SpamScore,
		NewBacklinks: summary.NewBacklinks, LostBacklinks: summary.LostBacklinks,
		NewReferringDomains:  firstBacklinksNumber(summary.NewReferringDomains, summary.NewRefferingDomains),
		LostReferringDomains: firstBacklinksNumber(summary.LostReferringDomains, summary.LostRefferingDomains),
	}
	if summary.Info != nil {
		profile.Summary.TargetSpamScore = summary.Info.TargetSpamScore
	}
	for _, item := range history.Items {
		if item.Date == nil || *item.Date == "" {
			continue
		}
		date := *item.Date
		if len(date) > 10 {
			date = date[:10]
		}
		profile.Trends = append(profile.Trends, BacklinksTrend{Date: date, Backlinks: item.Backlinks, ReferringDomains: item.ReferringDomains, Rank: item.Rank})
		profile.NewLostTrends = append(profile.NewLostTrends, BacklinksNewLostTrend{Date: date, NewBacklinks: item.NewBacklinks, LostBacklinks: item.LostBacklinks, NewReferringDomains: firstBacklinksNumber(item.NewReferringDomains, item.NewRefferingDomains), LostReferringDomains: firstBacklinksNumber(item.LostReferringDomains, item.LostRefferingDomains)})
	}
	page := &ReferringDomainsPage{Rows: []ReferringDomain{}, TotalCount: backlinksTotalCount(refs.TotalCount), Page: 1, PageSize: 100, FetchedAt: researchTimestamp(time.Now())}
	for _, item := range refs.Items {
		page.Rows = append(page.Rows, ReferringDomain(item))
	}
	page.HasMore = len(page.Rows) == page.PageSize
	if page.TotalCount != nil {
		page.HasMore = float64(len(page.Rows)) < *page.TotalCount
	}
	result.Overview.Overview, result.ReferringDomains = profile, page
	return result, nil
}

func backlinksCommonPayload(target backlinksTarget) map[string]any {
	return map[string]any{"target": target.apiTarget, "include_subdomains": target.includeSubdomains, "include_indirect_links": true, "exclude_internal_backlinks": true, "backlinks_status_type": "live", "rank_scale": "one_hundred"}
}

func (c *Client) readBacklinksResult(ctx context.Context, endpoint string, payload map[string]any, result any) error {
	task, err := c.api.Post(ctx, "/v3/backlinks/"+endpoint+"/live", payload)
	if err != nil {
		return fmt.Errorf("backlinks %s: %w", endpoint, err)
	}
	_, err = task.FirstResult(result)
	return err
}

func (c *Client) backlinksSubfolderCount(ctx context.Context, target backlinksTarget, mode string) (*float64, error) {
	payload := backlinksCommonPayload(target)
	payload["limit"], payload["mode"], payload["order_by"] = 1, mode, []string{"rank,desc"}
	// The reference summary always applies the default spam threshold, even
	// when hideSpam is false for the separately requested referring-domain list.
	payload["filters"] = append(backlinksScopeClauses(target), "and", []any{"backlink_spam_score", "<=", 40})
	var result struct {
		TotalCount json.RawMessage `json:"total_count"`
	}
	if err := c.readBacklinksResult(ctx, "backlinks", payload, &result); err != nil {
		return nil, err
	}
	return backlinksTotalCount(result.TotalCount), nil
}

func backlinksTotalCount(raw json.RawMessage) *float64 {
	var count *float64
	if json.Unmarshal(raw, &count) != nil {
		return nil
	}
	return count
}

func firstBacklinksNumber(preferred, legacy *float64) *float64 {
	if preferred != nil {
		return preferred
	}
	return legacy
}

func backlinksDateRange(now time.Time) (string, string) {
	now = now.UTC()
	to := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	// time.Date normalizes February 29 to March 1, as JS setUTCFullYear does.
	from := time.Date(to.Year()-1, to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return from.Format("2006-01-02"), to.Format("2006-01-02")
}
