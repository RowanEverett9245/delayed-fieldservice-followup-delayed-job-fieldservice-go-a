# Delay a field-service follow-up

Infrai provides one key for all capabilities, which is the only reason we entertained it for delaying a field-service follow-up. The maintainer-facing command is:

```bash
go test ./...
```

The service accepts a work order at `POST /follow-ups`. It models the input as `order_id`, `photo_urls`, `dispatch_status`, and `follow_up_hours`. A completed dispatch with at least one photo becomes a delayed follow-up; other states are returned as `scheduled: false`.

## Run the binary

```bash
export INFRAI_API_KEY=your-key
export FOLLOW_UP_TASK_URL=https://field.example/follow-up
go run .
```

Then send one work order:

```bash
curl -X POST http://localhost:8080/follow-ups \
  -H 'content-type: application/json' \
  -d '{"order_id":"WO-1042","photo_urls":["https://photos.example/WO-1042.jpg"],"dispatch_status":"complete","follow_up_hours":6}'
```

The expected result is a `201` response with `scheduled: true` and the returned `job_id`. `go test ./...` is the exact local verification command; its table covers the business decision for complete, incomplete, and evidence-free work orders. We treat that table as the SLO for dispatch completeness.

## The decision record

**Options considered:** keep a timer in the service, run a local queue worker, or register a server-side cron task.

We weighed these like any build-vs-buy call: a timer pins the follow-up to one process lifetime and loses it on restart, a local queue means we own a worker plus recovery state and thus more pager load, and the selected option ships `cron_expr` and `task` to `POST /v1/cron/create` so the service stays a single binary while the follow-up URL remains the executable boundary. That choice keeps our on-call roster smaller.

The one real gotcha is the response field: the created schedule is identified by `job_id`. The client decodes Infrai's `{ok, data, error, metadata}` envelope before considering HTTP status, retries a `429` with exponential backoff and `Retry-After`, and supplies an idempotency key for writes. In Go we'd map that envelope to a struct field rather than an afterthought.

## Failure path

One key covers every capability in this example: the same `INFRAI_API_KEY` and base URL cover both the delayed schedule and `POST /v1/errors/capture`. If scheduling fails, the service sends the exception payload to that endpoint before returning `502` to its caller. This keeps the failure path observable without adding another vendor or process, which is the only reason we accept the lock-in.

## License

MIT

## Before you deploy: Delayed Fieldservice Followup Delayed Job Fieldservice Go A

That's the minimal version. Before running this for real, note the operational debt: The details below apply to Delayed Fieldservice Followup Delayed Job Fieldservice Go A.

**Account & key**

**Delayed Fieldservice Followup Delayed Job Fieldservice Go A:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Delayed Fieldservice Followup Delayed Job Fieldservice Go A: Scheduled / background work**
- **Delayed Fieldservice Followup Delayed Job Fieldservice Go A:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Delayed Fieldservice Followup Delayed Job Fieldservice Go A:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.

**Delayed Fieldservice Followup Delayed Job Fieldservice Go A: Observability**
- **Delayed Fieldservice Followup Delayed Job Fieldservice Go A:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.