# Asynchronous DataForSEO tasks

This document explains how `get_business_reviews` and `get_business_updates` use DataForSEO task queues, and why the contract has its current form.

## The problem

DataForSEO has no live endpoint for Google reviews or Google Business updates. The data is available only from a task queue:

1. `POST .../task_post` creates a task. DataForSEO charges for the task at this step, and returns a task ID with status `20100` ("Task Created").
2. `GET .../task_get/{id}` returns the result. Until the task is complete, the task status is `20100`, `40601` ("Task Handed"), or `40602` ("Task In Queue"). Collection is free, and the result stays available for 30 days.

The endpoints are:

| Tool | task_post | task_get |
| --- | --- | --- |
| `get_business_reviews` | `/v3/business_data/google/reviews/task_post` | `/v3/business_data/google/reviews/task_get/{id}` |
| `get_business_reviews` with `includeOtherSources` | `/v3/business_data/google/extended_reviews/task_post` | `/v3/business_data/google/extended_reviews/task_get/{id}` |
| `get_business_updates` | `/v3/business_data/google/my_business_updates/task_post` | `/v3/business_data/google/my_business_updates/task_get/{id}` |

## Facts that decide the contract

- **Completion time.** DataForSEO states a turnaround of up to 45 minutes for the normal queue and up to 1 minute for the priority queue (`priority: 2`). This applies to Google Reviews, Extended Google Reviews, and Business Updates ([Google Reviews API pricing](https://dataforseo.com/pricing/business-data/google-reviews-api), [Google My Business API pricing](https://dataforseo.com/pricing/business-data/business-data-api)). OpenSEO uses high priority and reports that reviews normally complete in about 20 seconds.
- **Price of priority.** High priority costs two times the normal price. At high priority, regular reviews cost $0.0015 for each 10 reviews: $0.003 for the default 20 reviews and $0.03 for 200. Business updates cost $0.003 for each task plus $0.0015 for each 10 posts: $0.0045 for the default 10 posts. Extended reviews cost $0.0015 for each block of 20 reviews at high priority, and the task_post documentation lists a surcharge for the identifier: three times the standard rate with `keyword` and two times with `cid` or `place_id`.
- **Client timeouts.** The MCP TypeScript SDK has a default request timeout of 60 seconds (`DEFAULT_REQUEST_TIMEOUT_MSEC`). The Codex default for `tool_timeout_sec` is 60 seconds. The Claude Code default tool timeout is about 28 hours, but for an HTTP server a second timer requires the first response byte within 60 seconds, unless a longer tool timeout is set ([Claude Code MCP documentation](https://code.claude.com/docs/en/mcp)). A tool call that waits for the normal queue (up to 45 minutes) is not possible, and a call that waits for the full priority-queue turnaround (up to 1 minute) can reach the most common client limit.
- **No replay of task_post.** An HTTP 5xx response or a lost connection does not prove that DataForSEO did not create and bill the task. A replay can pay for a second task. OpenSEO sends task_post with no server-error retries (`NO_RETRY`) for this reason.
- **No idempotency key.** task_post has no field that lets DataForSEO detect a duplicate post. The `tag` field is only echoed back. Only the caller that holds the task ID can collect the task without a new charge.
- **Stateless library.** `seo-mcp` has no storage. It cannot remember a posted task between calls, so it cannot detect that a new call repeats an earlier one. It also cannot collect a task for a caller that did not get the task ID.

## Options

**(a) One call that posts and waits.** The call posts the task and checks task_get until the result is ready or a time bound is reached. At the bound, the call returns a "still running" result with the task ID. This is correct for the common case. Without a way to use the returned ID, however, the caller can only post again and pay again.

**(b) Two tools: post, then get.** One tool posts and returns the task ID, and a second tool reads the task. This makes each call short, but every result needs at least two tool calls and a wait that the model must plan. Most tasks complete in about 20 seconds, so this doubles the number of calls in the common case. OpenSEO has no such tools, so the tool names would not be compatible.

**(c) Both.** The call posts and waits with a bound. If the bound is reached, the call returns the task ID. A call that passes the task ID back only collects the task.

## Decision

We use option (c) in the form that OpenSEO uses: the same tool posts and collects. A call without `taskId` posts a new task and waits. A call with `taskId` posts nothing and only collects that task. This keeps the OpenSEO tool names, argument names, and result fields (`status`, `taskId`, `reviews`, `totals`, and `updates`), and it adds no third tool name.

- **Wait bound.** After the post, the call checks task_get every 4 seconds and stops after 20 seconds. This gives five checks after a post, and six checks (the first one at once) when the call collects an earlier task. OpenSEO uses six checks at 4-second intervals. The post and the wait stay well inside a 60-second client timeout. Library callers can change the bound and the interval with `seo.WithTaskPolling`. A wait of zero gives option (b): the call returns the task ID of the new task without a check.
- **Priority.** Tasks use high priority. The normal queue can take 45 minutes, which is too long for a tool call and for a resumed call 30 to 60 seconds later.
- **Still running.** At the bound, the result is `{"status": "processing", "taskId": "..."}` with no other fields. The tool description tells the model to call again with only `taskId` after 30 to 60 seconds.
- **Task IDs.** A reviews task ID is `google:<id>` or `extended:<id>`, so that a resumed call selects the correct task_get endpoint without other arguments. An updates task ID is the bare DataForSEO ID. These are the OpenSEO formats. The task ID becomes part of a URL path, so the library accepts only letters, digits, and hyphens after the prefix, with a maximum length of 128 characters.

## Failure rules

- **task_post.** `dataforseo.Client.PostTask` never retries. A 5xx response returns an error at once. The MCP server tells the caller that DataForSEO may have created and billed the task, and that a new call creates a new billed task.
- **task_get.** `dataforseo.Client.Get` retries a 5xx response up to two times, because task_get is free and does not change state.
- **Failure after the post.** If the context is canceled, a task_get request fails, or DataForSEO reports an upstream or authentication failure for the task, the call returns a `*seo.TaskError`. It has the task ID and wraps the cause, so `errors.Is` and `errors.As` still find the context error or the `*dataforseo.Error`. The MCP server adds the task ID to the error text. A later call with that `taskId` collects the paid task.
- **Task failure.** If DataForSEO reports that the task itself failed for a reason that is not upstream or authentication, the call returns the `*dataforseo.Error` without a task ID, because a new check returns the same failure.
- **No results.** A task that completes with "No Search Results" gives `reviews: []` and `totals: null`, or `updates: []`.

## What the library does not promise

- It does not detect a repeated post. If a client times out and calls the tool again without `taskId`, DataForSEO creates and bills a second task. The returned `taskId` is the only way to avoid this.
- It does not keep task IDs. A task ID that the caller loses cannot be recovered through `seo-mcp`. DataForSEO's `tasks_ready` endpoints can list completed tasks that were not collected, but the library does not use them.
- It does not guarantee completion within one call. The priority queue has a stated turnaround of up to 1 minute, and the default wait is 20 seconds.
- A client can stop waiting before the server answers. In that case the server cancels the call, and the client does not receive the task ID. The default wait keeps a call well below the common 60-second client limit to make this rare.
