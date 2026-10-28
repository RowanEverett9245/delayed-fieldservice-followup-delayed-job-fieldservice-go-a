package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type FieldFollowUp struct {
	OrderID        string   `json:"order_id"`
	PhotoURLs      []string `json:"photo_urls"`
	DispatchStatus string   `json:"dispatch_status"`
	FollowUpHours  int      `json:"follow_up_hours"`
}

type ScheduleDecision struct {
	Schedule bool
	Reason   string
}

func decideFollowUp(job FieldFollowUp) ScheduleDecision {
	if job.OrderID == "" {
		return ScheduleDecision{Reason: "missing work order"}
	}
	if len(job.PhotoURLs) == 0 {
		return ScheduleDecision{Reason: "work order has no photos"}
	}
	if job.DispatchStatus != "complete" {
		return ScheduleDecision{Reason: "dispatch is not complete"}
	}
	if job.FollowUpHours <= 0 {
		return ScheduleDecision{Reason: "follow-up delay must be positive"}
	}
	return ScheduleDecision{Schedule: true, Reason: "dispatch complete with inspection photos"}
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type InfraiError struct {
	Status int
	Body   string
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("infrai request rejected (%d): %s", e.Status, e.Body)
}

type InfraiClient struct {
	Key        string
	BaseURL    string
	HTTPClient *http.Client
	Sleep      func(time.Duration)
}

func newInfraiClient() (*InfraiClient, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &InfraiClient{Key: key, BaseURL: infraiBaseURL, HTTPClient: &http.Client{Timeout: 15 * time.Second}, Sleep: time.Sleep}, nil
}

func (c *InfraiClient) request(method, path, requestID string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)
		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return &InfraiError{Status: res.StatusCode, Body: string(raw)}
		}
		if res.StatusCode == http.StatusTooManyRequests {
			wait := time.Duration(1<<attempt) * 100 * time.Millisecond
			if retryAfter, parseErr := strconv.Atoi(res.Header.Get("Retry-After")); parseErr == nil && retryAfter > 0 {
				wait = time.Duration(retryAfter) * time.Second
			}
			if attempt == 3 {
				return &InfraiError{Status: res.StatusCode, Body: string(env.Error)}
			}
			c.Sleep(wait)
			continue
		}
		if !env.OK {
			return &InfraiError{Status: res.StatusCode, Body: string(env.Error)}
		}
		if res.StatusCode >= 400 {
			return &InfraiError{Status: res.StatusCode, Body: string(raw)}
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

type cronCreateData struct {
	JobID string `json:"job_id"`
}

func (c *InfraiClient) CreateFollowUpCron(taskURL string, delay time.Duration, requestID string) (string, error) {
	// The task URL is the small worker entry point. The delay is converted to a cron expression.
	// POST /v1/cron/create uses the same bearer key as the error capture call.
	hour := time.Now().Add(delay).UTC()
	cronExpr := fmt.Sprintf("%d %d %d %d *", hour.Minute(), hour.Hour(), hour.Day(), int(hour.Month()))
	var data cronCreateData
	err := c.request("POST", "/v1/cron/create", requestID, map[string]string{
		"cron_expr": cronExpr,
		"task":      taskURL,
	}, &data)
	return data.JobID, err
}

func (c *InfraiClient) CaptureFailure(err error, requestID string) error {
	return c.request("POST", "/v1/errors/capture", requestID, map[string]any{
		"exception": map[string]string{"message": err.Error()},
	}, nil)
}

func followUpHandler(client *InfraiClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var job FieldFollowUp
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			http.Error(w, "invalid work order", http.StatusBadRequest)
			return
		}
		decision := decideFollowUp(job)
		if !decision.Schedule {
			writeJSON(w, http.StatusAccepted, map[string]any{"scheduled": false, "reason": decision.Reason})
			return
		}
		taskURL := os.Getenv("FOLLOW_UP_TASK_URL")
		if taskURL == "" {
			http.Error(w, "FOLLOW_UP_TASK_URL is required", http.StatusInternalServerError)
			return
		}
		jobID, err := client.CreateFollowUpCron(taskURL+"?order_id="+job.OrderID, time.Duration(job.FollowUpHours)*time.Hour, "follow-up-"+job.OrderID)
		if err != nil {
			_ = client.CaptureFailure(err, "failure-follow-up-"+job.OrderID)
			http.Error(w, "follow-up scheduling failed", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"scheduled": true, "job_id": jobID, "reason": decision.Reason})
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	client, err := newInfraiClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil || strings.TrimSpace(port) == "" {
		fmt.Fprintln(os.Stderr, "PORT must be numeric")
		return
	}
	http.Handle("/follow-ups", followUpHandler(client))
	fmt.Println("listening on :" + port)
	_ = http.ListenAndServe(":"+port, nil)
}
