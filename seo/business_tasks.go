package seo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

// Google reviews and Google Business updates exist only as DataForSEO task
// queues: task_post creates a billed task and task_get collects it for free
// for 30 days. docs/async-tasks.md explains the contract below.
const (
	pathReviewsTaskPost         = "/v3/business_data/google/reviews/task_post"
	pathExtendedReviewsTaskPost = "/v3/business_data/google/extended_reviews/task_post"
	pathUpdatesTaskPost         = "/v3/business_data/google/my_business_updates/task_post"

	endpointReviews         = "reviews"
	endpointExtendedReviews = "extended_reviews"
	endpointUpdates         = "my_business_updates"

	reviewsTaskPrefix         = "google:"
	extendedReviewsTaskPrefix = "extended:"

	// High priority costs twice the normal price. DataForSEO states a
	// turnaround of up to 1 minute for it, against up to 45 minutes for
	// normal priority, which no tool call can wait for.
	taskPriorityHigh = 2

	maxTaskIDLength = 128
)

// Default task polling. Six checks four seconds apart match OpenSEO, and
// the post plus the wait stay well inside the 60-second default tool timeout
// of the MCP TypeScript SDK and Codex.
const (
	DefaultTaskWait         = 20 * time.Second
	DefaultTaskPollInterval = 4 * time.Second
)

// Status values of a queued-task result.
const (
	TaskCompleted  = "completed"
	TaskProcessing = "processing"
)

// TaskError is a failure after DataForSEO created a task, for example a
// canceled context or a failed task_get request. DataForSEO charged for the
// task when it created it. A later call with TaskID collects the task without
// a new charge; a call without TaskID posts and pays for a new task.
type TaskError struct {
	// TaskID is the value to pass back as the request's TaskID.
	TaskID string
	Err    error
}

func (e *TaskError) Error() string {
	return fmt.Sprintf("%v (DataForSEO task %q was created; pass it as taskId to collect it without a new charge)", e.Err, e.TaskID)
}

func (e *TaskError) Unwrap() error { return e.Err }

// BusinessReviewsRequest selects a business and the reviews to collect.
// Exactly one of BusinessName, CID and PlaceID is required unless TaskID is
// set. With TaskID, the call only collects that earlier task and ignores the
// other fields. Near takes precedence over LocationCode. Nil Depth means 20;
// the range is 10 to 200. SortBy is newest (default), highest_rating,
// lowest_rating or relevant; the extended endpoint, which IncludeOtherSources
// selects, cannot sort and ignores it.
type BusinessReviewsRequest struct {
	BusinessName        *string              `json:"businessName,omitempty"`
	CID                 *string              `json:"cid,omitempty"`
	PlaceID             *string              `json:"placeId,omitempty"`
	Near                *BusinessProfileNear `json:"near,omitempty"`
	LocationCode        int                  `json:"locationCode,omitempty"`
	LanguageCode        string               `json:"languageCode,omitempty"`
	Depth               *int                 `json:"depth,omitempty"`
	SortBy              string               `json:"sortBy,omitempty"`
	IncludeOtherSources bool                 `json:"includeOtherSources,omitempty"`
	TaskID              string               `json:"taskId,omitempty"`
}

// BusinessReviewsResult is a collected review list, or a task that is still
// running. When Status is TaskProcessing, only Status and TaskID are set and
// encoded. TaskID has the form "google:<id>" or "extended:<id>".
type BusinessReviewsResult struct {
	Status  string                       `json:"status"`
	TaskID  string                       `json:"taskId"`
	Reviews []map[string]json.RawMessage `json:"reviews"`
	// Totals has title, reviews_count, rating, cid and place_id of the
	// business, each null when DataForSEO does not return it. It is null
	// when the task found no business or no reviews.
	Totals map[string]json.RawMessage `json:"totals"`
}

// MarshalJSON leaves out reviews and totals while the task is running.
func (r BusinessReviewsResult) MarshalJSON() ([]byte, error) {
	if r.Status == TaskProcessing {
		return json.Marshal(pendingTask{Status: r.Status, TaskID: r.TaskID})
	}
	type completed BusinessReviewsResult
	return json.Marshal(completed(r))
}

// BusinessUpdatesRequest selects a business and the number of Google Business
// posts to collect. The identifier, location and TaskID rules are those of
// BusinessReviewsRequest. Nil Depth means 10; the range is 10 to 100.
type BusinessUpdatesRequest struct {
	BusinessName *string              `json:"businessName,omitempty"`
	CID          *string              `json:"cid,omitempty"`
	PlaceID      *string              `json:"placeId,omitempty"`
	Near         *BusinessProfileNear `json:"near,omitempty"`
	LocationCode int                  `json:"locationCode,omitempty"`
	LanguageCode string               `json:"languageCode,omitempty"`
	Depth        *int                 `json:"depth,omitempty"`
	TaskID       string               `json:"taskId,omitempty"`
}

// BusinessUpdatesResult is a collected list of Google Business posts, or a
// task that is still running. When Status is TaskProcessing, only Status and
// TaskID are set and encoded.
type BusinessUpdatesResult struct {
	Status  string                       `json:"status"`
	TaskID  string                       `json:"taskId"`
	Updates []map[string]json.RawMessage `json:"updates"`
}

// MarshalJSON leaves out updates while the task is running.
func (r BusinessUpdatesResult) MarshalJSON() ([]byte, error) {
	if r.Status == TaskProcessing {
		return json.Marshal(pendingTask{Status: r.Status, TaskID: r.TaskID})
	}
	type completed BusinessUpdatesResult
	return json.Marshal(completed(r))
}

type pendingTask struct {
	Status string `json:"status"`
	TaskID string `json:"taskId"`
}

// Review rows carry long review, avatar and image URLs and xpaths; these are
// the fields that review analysis reads, as in OpenSEO.
var businessReviewFields = []string{
	"rank_absolute", "time_ago", "timestamp", "rating", "review_text",
	"original_review_text", "original_language", "profile_name", "local_guide",
	"reviews_count", "photos_count", "review_highlights", "source", "owner_answer",
	"owner_time_ago", "owner_timestamp", "review_id",
}

var businessReviewTotalFields = []string{"title", "reviews_count", "rating", "cid", "place_id"}

var businessUpdateFields = []string{
	"rank_absolute", "author", "post_date", "timestamp", "post_text", "snippet", "url", "links",
}

// BusinessReviews posts a Google reviews task at high priority and waits for
// it as WithTaskPolling sets. DataForSEO charges when it creates the task and
// for each block of 10 returned reviews (20 with IncludeOtherSources);
// collecting a task with TaskID costs nothing. The task is never posted twice:
// a failure after the post returns a *TaskError with the task ID.
func (c *Client) BusinessReviews(ctx context.Context, req BusinessReviewsRequest) (*BusinessReviewsResult, error) {
	if req.TaskID != "" {
		endpoint, id, err := parseReviewsTaskID(req.TaskID)
		if err != nil {
			return nil, err
		}
		return c.collectReviews(ctx, endpoint, id, req.TaskID, false)
	}
	if _, err := businessIdentifierKeyword(req.BusinessName, req.CID, req.PlaceID); err != nil {
		return nil, err
	}
	if req.LocationCode < 0 {
		return nil, inputErrorf("locationCode must be positive")
	}
	language, err := c.businessLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	depth, err := taskDepth(req.Depth, 20, 200)
	if err != nil {
		return nil, err
	}
	sortBy := req.SortBy
	switch sortBy {
	case "":
		sortBy = "newest"
	case "newest", "highest_rating", "lowest_rating", "relevant":
	default:
		return nil, inputErrorf("sortBy must be newest, highest_rating, lowest_rating or relevant")
	}
	task := map[string]any{"language_code": language, "depth": depth, "priority": taskPriorityHigh}
	// The reviews endpoints take the three identifiers as separate fields.
	for field, value := range map[string]*string{"keyword": req.BusinessName, "cid": req.CID, "place_id": req.PlaceID} {
		if value != nil {
			task[field] = *value
		}
	}
	if err := c.setBusinessDataLocation(task, req.Near, req.LocationCode); err != nil {
		return nil, err
	}
	path, endpoint, prefix := pathReviewsTaskPost, endpointReviews, reviewsTaskPrefix
	if req.IncludeOtherSources {
		path, endpoint, prefix = pathExtendedReviewsTaskPost, endpointExtendedReviews, extendedReviewsTaskPrefix
	} else {
		task["sort_by"] = sortBy
	}
	posted, err := c.api.PostTask(ctx, path, task)
	if err != nil {
		return nil, err
	}
	return c.collectReviews(ctx, endpoint, posted.ID, prefix+posted.ID, true)
}

func (c *Client) collectReviews(ctx context.Context, endpoint, id, publicID string, posted bool) (*BusinessReviewsResult, error) {
	first, done, err := c.waitForTask(ctx, endpoint, id, publicID, posted)
	if err != nil {
		return nil, err
	}
	if !done {
		return &BusinessReviewsResult{Status: TaskProcessing, TaskID: publicID}, nil
	}
	result := &BusinessReviewsResult{Status: TaskCompleted, TaskID: publicID, Reviews: taskItems(first, businessReviewFields)}
	if first != nil {
		result.Totals = make(map[string]json.RawMessage, len(businessReviewTotalFields))
		for _, field := range businessReviewTotalFields {
			result.Totals[field] = json.RawMessage("null")
			if value, ok := first[field]; ok {
				result.Totals[field] = value
			}
		}
	}
	return result, nil
}

// BusinessUpdates posts a Google Business updates task at high priority and
// waits for it as WithTaskPolling sets. DataForSEO charges when it creates the
// task and for each block of 10 returned posts; collecting a task with TaskID
// costs nothing. The task is never posted twice: a failure after the post
// returns a *TaskError with the task ID.
func (c *Client) BusinessUpdates(ctx context.Context, req BusinessUpdatesRequest) (*BusinessUpdatesResult, error) {
	if req.TaskID != "" {
		if strings.Contains(req.TaskID, ":") {
			return nil, inputErrorf("That looks like a get_business_reviews taskId; pass the bare taskId this tool returned.")
		}
		if !validTaskID(req.TaskID) {
			return nil, inputErrorf("taskId must be the value this tool returned.")
		}
		return c.collectUpdates(ctx, req.TaskID, false)
	}
	keyword, err := businessIdentifierKeyword(req.BusinessName, req.CID, req.PlaceID)
	if err != nil {
		return nil, err
	}
	if req.LocationCode < 0 {
		return nil, inputErrorf("locationCode must be positive")
	}
	language, err := c.businessLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	depth, err := taskDepth(req.Depth, 10, 100)
	if err != nil {
		return nil, err
	}
	// my_business_updates takes only keyword, with cid: and place_id: prefixes
	// for the other identifiers.
	task := map[string]any{"keyword": keyword, "language_code": language, "depth": depth, "priority": taskPriorityHigh}
	if err := c.setBusinessDataLocation(task, req.Near, req.LocationCode); err != nil {
		return nil, err
	}
	posted, err := c.api.PostTask(ctx, pathUpdatesTaskPost, task)
	if err != nil {
		return nil, err
	}
	return c.collectUpdates(ctx, posted.ID, true)
}

func (c *Client) collectUpdates(ctx context.Context, id string, posted bool) (*BusinessUpdatesResult, error) {
	first, done, err := c.waitForTask(ctx, endpointUpdates, id, id, posted)
	if err != nil {
		return nil, err
	}
	if !done {
		return &BusinessUpdatesResult{Status: TaskProcessing, TaskID: id}, nil
	}
	return &BusinessUpdatesResult{Status: TaskCompleted, TaskID: id, Updates: taskItems(first, businessUpdateFields)}, nil
}

// waitForTask checks a task with task_get until it completes or the wait ends.
// A task posted by this call is first checked one interval after the post; an
// earlier task is checked at once. It returns done=false when the task is
// still running at the end of the wait, and the first result entry when it
// completed (nil for a task that found no results).
func (c *Client) waitForTask(ctx context.Context, endpoint, id, publicID string, posted bool) (map[string]json.RawMessage, bool, error) {
	path := "/v3/business_data/google/" + endpoint + "/task_get/" + id
	start := time.Now()
	for check := 0; ; check++ {
		offset := time.Duration(check) * c.taskInterval
		if posted {
			offset += c.taskInterval
		}
		if check > 0 || posted {
			if offset > c.taskWait {
				return nil, false, nil
			}
			timer := time.NewTimer(time.Until(start.Add(offset)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, false, &TaskError{TaskID: publicID, Err: ctx.Err()}
			case <-timer.C:
			}
		}
		task, err := c.api.Get(ctx, path)
		var provider *dataforseo.Error
		switch {
		case err == nil:
			var entries []json.RawMessage
			if len(task.Result) > 0 {
				if err := json.Unmarshal(task.Result, &entries); err != nil {
					return nil, false, &TaskError{TaskID: publicID, Err: fmt.Errorf("business task: decode result: %w", err)}
				}
			}
			if len(entries) == 0 {
				return nil, true, nil
			}
			return businessObject(entries[0]), true, nil
		case errors.As(err, &provider) && provider.InProgress():
			continue
		case dataforseo.IsNoResults(err):
			return nil, true, nil
		case errors.As(err, &provider) && provider.HTTPStatus == 0 && !provider.Upstream() && !provider.AuthFailed():
			// DataForSEO reported that the task itself failed. Collecting it
			// again returns the same failure, so no task ID is offered.
			return nil, false, err
		default:
			return nil, false, &TaskError{TaskID: publicID, Err: err}
		}
	}
}

// taskItems projects the items of a task result entry onto fields. A missing
// entry or items list gives an empty list.
func taskItems(first map[string]json.RawMessage, fields []string) []map[string]json.RawMessage {
	rows := []map[string]json.RawMessage{}
	var items []json.RawMessage
	if json.Unmarshal(first["items"], &items) != nil {
		return rows
	}
	for _, item := range items {
		rows = append(rows, pickBusinessFields(businessObject(item), fields))
	}
	return rows
}

func taskDepth(value *int, defaultValue, maximum int) (int, error) {
	if value == nil {
		return defaultValue, nil
	}
	if *value < 10 || *value > maximum {
		return 0, inputErrorf("depth must be between 10 and %d", maximum)
	}
	return *value, nil
}

func parseReviewsTaskID(taskID string) (endpoint, id string, err error) {
	for prefix, endpoint := range map[string]string{reviewsTaskPrefix: endpointReviews, extendedReviewsTaskPrefix: endpointExtendedReviews} {
		if id, ok := strings.CutPrefix(taskID, prefix); ok && len(taskID) <= maxTaskIDLength && validTaskID(id) {
			return endpoint, id, nil
		}
	}
	return "", "", inputErrorf(`taskId must be the value this tool returned, formatted as "google:<id>" or "extended:<id>".`)
}

// validTaskID accepts the characters of a DataForSEO task ID (a UUID). The ID
// becomes part of the task_get URL path, so nothing else may pass.
func validTaskID(id string) bool {
	if id == "" || len(id) > maxTaskIDLength {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
		default:
			return false
		}
	}
	return true
}
