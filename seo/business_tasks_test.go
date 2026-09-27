package seo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

const taskTestID = "09271200-1535-0161-0000-772a4543ee8a"

// taskTestGet is one scripted task_get answer: an HTTP error status, or a
// task status with its message and result.
type taskTestGet struct {
	httpStatus int
	status     int
	message    string
	result     string
}

var (
	taskTestQueued = taskTestGet{status: 40602, message: "Task In Queue."}
	taskTestReady  = func(result string) taskTestGet { return taskTestGet{status: 20000, message: "Ok.", result: result} }
)

// taskTestFake serves task_post and task_get for one endpoint. After the
// scripted answers run out, task_get repeats the last one.
type taskTestFake struct {
	t        *testing.T
	postPath string
	getPath  string
	postBody string
	postHTTP int
	gets     []taskTestGet
	onGet    func(n int)
	posts    atomic.Int32
	getCount atomic.Int32
}

func (f *taskTestFake) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == f.postPath:
		f.posts.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Error(err)
		}
		if f.postBody != "" {
			domainTestJSON(f.t, json.RawMessage(raw), "["+f.postBody+"]")
		}
		if f.postHTTP != 0 {
			http.Error(w, "upstream failure", f.postHTTP)
			return
		}
		_, _ = io.WriteString(w, `{"status_code":20000,"tasks":[{"id":"`+taskTestID+`","status_code":20100,"status_message":"Task Created.","cost":0.003,"result":null}]}`)
	case r.Method == http.MethodGet && r.URL.Path == f.getPath:
		n := int(f.getCount.Add(1))
		if f.onGet != nil {
			f.onGet(n)
		}
		answer := f.gets[min(n, len(f.gets))-1]
		if answer.httpStatus != 0 {
			http.Error(w, "upstream failure", answer.httpStatus)
			return
		}
		result := answer.result
		if result == "" {
			result = "null"
		}
		_, _ = io.WriteString(w, `{"status_code":20000,"tasks":[{"id":"`+taskTestID+`","status_code":`+strconv.Itoa(answer.status)+`,"status_message":"`+answer.message+`","cost":0,"result":`+result+`}]}`)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

const (
	taskTestReviewsResult = `[{"title":"Example Cafe","reviews_count":120,"rating":{"value":4.5,"votes_count":120},"cid":"123","place_id":"place","check_url":"ignored","items":[{"rank_absolute":1,"rating":{"value":5},"review_text":"Good","profile_name":"A","owner_answer":null,"review_url":"ignored","xpath":"ignored"}]}]`
	taskTestReviewsWant   = `{"status":"completed","taskId":"google:` + taskTestID + `","reviews":[{"rank_absolute":1,"rating":{"value":5},"review_text":"Good","profile_name":"A","owner_answer":null}],"totals":{"title":"Example Cafe","reviews_count":120,"rating":{"value":4.5,"votes_count":120},"cid":"123","place_id":"place"}}`
	taskTestUpdatesResult = `[{"items":[{"rank_absolute":1,"post_text":"Open late","post_date":"2026-09-01","images_url":"ignored","links":[{"url":"https://example.com/"}]}]}]`
)

func TestBusinessTasks(t *testing.T) {
	fast := WithTaskPolling(50*time.Millisecond, 10*time.Millisecond)
	for _, tt := range []struct {
		name     string
		req      any
		postPath string
		getPath  string
		postBody string
		postHTTP int
		gets     []taskTestGet
		opts     []Option
		want     string
		wantErr  func(*testing.T, error)
		posts    int32
		minGets  int32
		maxGets  int32
	}{
		{
			name: "reviews posted then ready", req: BusinessReviewsRequest{BusinessName: domainTestPtr("Example Cafe")},
			postPath: pathReviewsTaskPost, getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
			postBody: `{"keyword":"Example Cafe","location_code":2840,"language_code":"en","depth":20,"sort_by":"newest","priority":2}`,
			gets:     []taskTestGet{taskTestQueued, {status: 40601, message: "Task Handed."}, taskTestReady(taskTestReviewsResult)},
			want:     taskTestReviewsWant, posts: 1, minGets: 3, maxGets: 3,
		},
		{
			name: "extended reviews by cid near a coordinate", req: BusinessReviewsRequest{
				CID: domainTestPtr("123"), Near: &BusinessProfileNear{Latitude: domainTestPtr(33.0), Longitude: domainTestPtr(-84.0)},
				LanguageCode: "de", Depth: domainTestPtr(200), SortBy: "lowest_rating", IncludeOtherSources: true,
			},
			postPath: pathExtendedReviewsTaskPost, getPath: "/v3/business_data/google/extended_reviews/task_get/" + taskTestID,
			postBody: `{"cid":"123","location_coordinate":"33,-84,10000","language_code":"de","depth":200,"priority":2}`,
			gets:     []taskTestGet{taskTestReady(`[{"items":[{"review_text":"Fine","source":{"title":"Example Reviews"}}]}]`)},
			want:     `{"status":"completed","taskId":"extended:` + taskTestID + `","reviews":[{"review_text":"Fine","source":{"title":"Example Reviews"}}],"totals":{"title":null,"reviews_count":null,"rating":null,"cid":null,"place_id":null}}`,
			posts:    1, minGets: 1, maxGets: 1,
		},
		{
			name: "reviews still running at the bound", req: BusinessReviewsRequest{PlaceID: domainTestPtr("place"), LocationCode: 2276, SortBy: "relevant"},
			postPath: pathReviewsTaskPost, getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
			postBody: `{"place_id":"place","location_code":2276,"language_code":"en","depth":20,"sort_by":"relevant","priority":2}`,
			gets:     []taskTestGet{taskTestQueued},
			want:     `{"status":"processing","taskId":"google:` + taskTestID + `"}`,
			// Checks at 10, 20, 30, 40 and 50 ms after the post.
			posts: 1, minGets: 5, maxGets: 5,
		},
		{
			name: "reviews collected by task ID", req: BusinessReviewsRequest{TaskID: "google:" + taskTestID, BusinessName: domainTestPtr("ignored"), Depth: domainTestPtr(1)},
			getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
			gets:    []taskTestGet{taskTestReady(taskTestReviewsResult)},
			want:    taskTestReviewsWant, posts: 0, minGets: 1, maxGets: 1,
		},
		{
			name: "reviews with no results", req: BusinessReviewsRequest{TaskID: "google:" + taskTestID},
			getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
			gets:    []taskTestGet{{status: 40102, message: "No Search Results."}},
			want:    `{"status":"completed","taskId":"google:` + taskTestID + `","reviews":[],"totals":null}`,
			minGets: 1, maxGets: 1,
		},
		{
			name: "zero wait returns the new task at once", req: BusinessUpdatesRequest{CID: domainTestPtr("123")},
			postPath: pathUpdatesTaskPost, getPath: "/v3/business_data/google/my_business_updates/task_get/" + taskTestID,
			postBody: `{"keyword":"cid:123","location_code":2840,"language_code":"en","depth":10,"priority":2}`,
			gets:     []taskTestGet{taskTestQueued}, opts: []Option{WithTaskPolling(0, time.Hour)},
			want:  `{"status":"processing","taskId":"` + taskTestID + `"}`,
			posts: 1, minGets: 0, maxGets: 0,
		},
		{
			name: "updates posted then ready", req: BusinessUpdatesRequest{PlaceID: domainTestPtr("place"), Near: &BusinessProfileNear{Latitude: domainTestPtr(1.0), Longitude: domainTestPtr(2.0), RadiusKM: domainTestPtr(0.2)}, Depth: domainTestPtr(100)},
			postPath: pathUpdatesTaskPost, getPath: "/v3/business_data/google/my_business_updates/task_get/" + taskTestID,
			postBody: `{"keyword":"place_id:place","location_coordinate":"1,2,200","language_code":"en","depth":100,"priority":2}`,
			gets:     []taskTestGet{taskTestQueued, taskTestReady(taskTestUpdatesResult)},
			want:     `{"status":"completed","taskId":"` + taskTestID + `","updates":[{"rank_absolute":1,"post_text":"Open late","post_date":"2026-09-01","links":[{"url":"https://example.com/"}]}]}`,
			posts:    1, minGets: 2, maxGets: 2,
		},
		{
			name: "updates with an empty result", req: BusinessUpdatesRequest{TaskID: taskTestID},
			getPath: "/v3/business_data/google/my_business_updates/task_get/" + taskTestID,
			gets:    []taskTestGet{taskTestReady(`[{"items":null}]`)},
			want:    `{"status":"completed","taskId":"` + taskTestID + `","updates":[]}`,
			minGets: 1, maxGets: 1,
		},
		{
			name: "task failure is final", req: BusinessUpdatesRequest{BusinessName: domainTestPtr("Example Cafe")},
			postPath: pathUpdatesTaskPost, getPath: "/v3/business_data/google/my_business_updates/task_get/" + taskTestID,
			gets: []taskTestGet{{status: 40401, message: "Task Not Found."}},
			wantErr: func(t *testing.T, err error) {
				var provider *dataforseo.Error
				var taskErr *TaskError
				if !errors.As(err, &provider) || provider.StatusCode != 40401 || errors.As(err, &taskErr) {
					t.Errorf("error = %v, want the task failure without a task ID", err)
				}
			},
			posts: 1, minGets: 1, maxGets: 1,
		},
		{
			name: "upstream task_get failure keeps the task ID", req: BusinessReviewsRequest{BusinessName: domainTestPtr("Example Cafe")},
			postPath: pathReviewsTaskPost, getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
			gets: []taskTestGet{{httpStatus: http.StatusServiceUnavailable}},
			wantErr: func(t *testing.T, err error) {
				var provider *dataforseo.Error
				var taskErr *TaskError
				if !errors.As(err, &taskErr) || taskErr.TaskID != "google:"+taskTestID || !errors.As(err, &provider) || provider.HTTPStatus != 503 {
					t.Errorf("error = %v, want a TaskError around HTTP 503", err)
				}
				if !strings.Contains(err.Error(), "google:"+taskTestID) {
					t.Errorf("message %q has no task ID", err)
				}
			},
			// task_get is free and idempotent, so the transport retries it.
			posts: 1, minGets: 3, maxGets: 3,
		},
		{
			name: "5xx on task_post is not retried", req: BusinessReviewsRequest{BusinessName: domainTestPtr("Example Cafe")},
			postPath: pathReviewsTaskPost, getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID, postHTTP: http.StatusInternalServerError,
			wantErr: func(t *testing.T, err error) {
				var provider *dataforseo.Error
				var taskErr *TaskError
				if !errors.As(err, &provider) || provider.HTTPStatus != 500 || errors.As(err, &taskErr) {
					t.Errorf("error = %v, want HTTP 500 without a task ID", err)
				}
			},
			posts: 1, minGets: 0, maxGets: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &taskTestFake{t: t, postPath: tt.postPath, getPath: tt.getPath, postBody: tt.postBody, postHTTP: tt.postHTTP, gets: tt.gets}
			opts := append([]Option{fast}, tt.opts...)
			c := domainTestClient(t, fake.handler, opts...)
			var got any
			var err error
			switch req := tt.req.(type) {
			case BusinessReviewsRequest:
				got, err = c.BusinessReviews(t.Context(), req)
			case BusinessUpdatesRequest:
				got, err = c.BusinessUpdates(t.Context(), req)
			}
			if tt.wantErr != nil {
				tt.wantErr(t, err)
			} else if err != nil {
				t.Fatal(err)
			} else {
				domainTestJSON(t, got, tt.want)
			}
			if posts := fake.posts.Load(); posts != tt.posts {
				t.Errorf("task_post requests = %d, want %d", posts, tt.posts)
			}
			if gets := fake.getCount.Load(); gets < tt.minGets || gets > tt.maxGets {
				t.Errorf("task_get requests = %d, want %d to %d", gets, tt.minGets, tt.maxGets)
			}
		})
	}
}

func TestBusinessTaskCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fake := &taskTestFake{
		t: t, postPath: pathReviewsTaskPost, getPath: "/v3/business_data/google/reviews/task_get/" + taskTestID,
		gets: []taskTestGet{taskTestQueued},
		// Cancel while the call waits for its second check.
		onGet: func(n int) {
			if n == 1 {
				cancel()
			}
		},
	}
	c := domainTestClient(t, fake.handler, WithTaskPolling(time.Hour, 10*time.Millisecond))
	start := time.Now()
	_, err := c.BusinessReviews(ctx, BusinessReviewsRequest{BusinessName: domainTestPtr("Example Cafe")})
	var taskErr *TaskError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &taskErr) || taskErr.TaskID != "google:"+taskTestID {
		t.Fatalf("error = %v, want a canceled TaskError with the task ID", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("cancel took %s", elapsed)
	}
	if posts, gets := fake.posts.Load(), fake.getCount.Load(); posts != 1 || gets != 1 {
		t.Errorf("requests: %d task_post, %d task_get; want 1 and 1", posts, gets)
	}
}

func TestBusinessTaskInputErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  any
		want string
	}{
		{"no identifier", BusinessReviewsRequest{}, "exactly one business identifier"},
		{"two identifiers", BusinessUpdatesRequest{CID: domainTestPtr("1"), PlaceID: domainTestPtr("p")}, "exactly one business identifier"},
		{"reviews depth below minimum", BusinessReviewsRequest{CID: domainTestPtr("1"), Depth: domainTestPtr(9)}, "depth must be between 10 and 200"},
		{"reviews depth above maximum", BusinessReviewsRequest{CID: domainTestPtr("1"), Depth: domainTestPtr(201)}, "depth must be between 10 and 200"},
		{"updates depth above maximum", BusinessUpdatesRequest{CID: domainTestPtr("1"), Depth: domainTestPtr(101)}, "depth must be between 10 and 100"},
		{"unknown sort", BusinessReviewsRequest{CID: domainTestPtr("1"), SortBy: "oldest"}, "sortBy must be"},
		{"negative location", BusinessReviewsRequest{CID: domainTestPtr("1"), LocationCode: -1}, "locationCode must be positive"},
		{"radius too large", BusinessUpdatesRequest{CID: domainTestPtr("1"), Near: &BusinessProfileNear{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(0.0), RadiusKM: domainTestPtr(200.0)}}, "near.radiusKm must be between 0.2 and 199"},
		{"language", BusinessUpdatesRequest{CID: domainTestPtr("1"), LanguageCode: "xx"}, "language code"},
		{"reviews task ID without prefix", BusinessReviewsRequest{TaskID: taskTestID}, `formatted as "google:<id>"`},
		{"reviews task ID with a path", BusinessReviewsRequest{TaskID: "google:../../v3/other"}, `formatted as "google:<id>"`},
		{"reviews task ID too long", BusinessReviewsRequest{TaskID: "google:" + strings.Repeat("a", 122)}, `formatted as "google:<id>"`},
		{"updates task ID from reviews", BusinessUpdatesRequest{TaskID: "google:" + taskTestID}, "get_business_reviews taskId"},
		{"updates task ID with a path", BusinessUpdatesRequest{TaskID: "a/b"}, "taskId must be the value this tool returned"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
				t.Errorf("invalid input reached DataForSEO: %s", r.URL.Path)
			})
			var err error
			switch req := tt.req.(type) {
			case BusinessReviewsRequest:
				_, err = c.BusinessReviews(t.Context(), req)
			case BusinessUpdatesRequest:
				_, err = c.BusinessUpdates(t.Context(), req)
			}
			var input *InputError
			if !errors.As(err, &input) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want InputError containing %q", err, tt.want)
			}
		})
	}
}
