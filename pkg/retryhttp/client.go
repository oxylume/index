package retryhttp

import (
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultDelayBase = 1 * time.Second
	defaultDelayMax  = 20 * time.Second
	defaultMaxRetry  = 4
)

var DefaultClient = NewClient()

type Client struct {
	DelayBase time.Duration
	DelayMax  time.Duration
	MaxRetry  int

	client *http.Client
}

func NewClient() *Client {
	return &Client{
		DelayBase: defaultDelayBase,
		DelayMax:  defaultDelayMax,
		MaxRetry:  defaultMaxRetry,
		client:    http.DefaultClient,
	}
}

// req.Body must be nil (currently not supported for retry)
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// todo: implement retry with request body
	if req.Body != nil && req.Body != http.NoBody {
		panic("retryhttp is not supported with a request body")
	}

	var resp *http.Response
	var err error

	for retry := 0; ; retry++ {
		resp, err = c.client.Do(req)
		if !c.shouldRetry(resp, err) {
			break
		}

		if retry >= c.MaxRetry {
			break
		}

		if resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		delay := c.backoff(retry, resp)
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
	}
	return resp, err
}

func (c *Client) shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}

	if resp.StatusCode == 0 {
		return true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode >= http.StatusInternalServerError &&
		resp.StatusCode != http.StatusNotImplemented {
		return true
	}
	return false
}

func (c *Client) backoff(retry int, resp *http.Response) time.Duration {
	if delay, ok := parseRetryAfter(resp); ok {
		return delay
	}

	backoff := int64(c.DelayBase) * (1 << retry)
	// delay will be random in range [0;backoff)
	jitter := rand.Int64N(backoff)
	delay := time.Duration(jitter)
	if delay > c.DelayMax {
		delay = c.DelayMax
	}
	return delay
}

func parseRetryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}
	if resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0, false
	}

	if delay, err := strconv.ParseInt(header, 10, 64); err == nil {
		if delay < 0 {
			return 0, false
		}
		return time.Duration(delay) * time.Second, true
	}

	retryTime, err := time.Parse(time.RFC1123, header)
	if err != nil {
		return 0, false
	}
	until := time.Until(retryTime)
	if until < 0 {
		return 0, true
	}
	return until, true
}
