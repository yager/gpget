package gopro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one GoPro camera over its local HTTP API.
type Client struct {
	// BaseURL is like "http://172.23.188.51:8080" (no trailing slash).
	BaseURL string
	HTTP    *http.Client
}

// NewClient builds a Client for the given "host", "host:port" or full URL.
func NewClient(hostOrURL string) *Client {
	return &Client{
		BaseURL: normalizeBase(hostOrURL),
		HTTP: &http.Client{
			// Big ceiling: a 4GB+ file over ~20 MB/s can take minutes. Per-call
			// deadlines come from the context instead.
			Timeout: 6 * time.Hour,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
				ResponseHeaderTimeout: 20 * time.Second,
				DisableCompression:    true,
			},
		},
	}
}

func normalizeBase(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if !strings.Contains(s, "://") {
		if !strings.Contains(s, ":") {
			s += ":8080"
		}
		s = "http://" + s
	}
	return s
}

// Host returns the bare host (no scheme/port) of the client's base URL.
func (c *Client) Host() string {
	if u, err := url.Parse(c.BaseURL); err == nil {
		return u.Hostname()
	}
	return c.BaseURL
}

// HTTPError is a non-2xx response from the camera.
type HTTPError struct {
	Path   string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s: HTTP %d (%s)", e.Path, e.Status, e.Body)
	}
	return fmt.Sprintf("%s: HTTP %d", e.Path, e.Status)
}

// get issues a GET and returns the open response body (caller must Close it).
func (c *Client) get(ctx context.Context, path string, hdr http.Header) (io.ReadCloser, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, nil, err
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, resp, &HTTPError{Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	return resp.Body, resp, nil
}

// getJSON does a short-deadline GET and decodes the body into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, _, err := c.get(tctx, path, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode: %w", path, err)
	}
	return nil
}

// hit does a short-deadline GET and discards the body (for control endpoints).
func (c *Client) hit(ctx context.Context, path string) error {
	tctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	body, _, err := c.get(tctx, path, nil)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(body, 4096))
	body.Close()
	return nil
}

// ---- API methods -----------------------------------------------------------

// Info returns GET /gopro/camera/info.
func (c *Client) Info(ctx context.Context) (*CameraInfo, error) {
	var out CameraInfo
	if err := c.getJSON(ctx, "/gopro/camera/info", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// APIVersion returns GET /gopro/version (e.g. "2.0").
func (c *Client) APIVersion(ctx context.Context) (string, error) {
	var out Version
	if err := c.getJSON(ctx, "/gopro/version", &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

// EnableWiredControl sends wired_usb?p=1. Harmless if already on; some firmwares
// 404 it, which callers treat as non-fatal.
func (c *Client) EnableWiredControl(ctx context.Context) error {
	return c.hit(ctx, "/gopro/camera/control/wired_usb?p=1")
}

// KeepAlive sends GET /gopro/camera/keep_alive.
func (c *Client) KeepAlive(ctx context.Context) error {
	return c.hit(ctx, "/gopro/camera/keep_alive")
}

// DateTime is GET /gopro/camera/get_date_time: the camera's wall clock plus its
// configured timezone offset (minutes) and DST flag.
type DateTime struct {
	Date  string `json:"date"` // "2026_09_04"
	Time  string `json:"time"` // "20_50_19"
	TZone int    `json:"tzone"`
	DST   int    `json:"dst"`
}

// GetDateTime returns GET /gopro/camera/get_date_time.
func (c *Client) GetDateTime(ctx context.Context) (*DateTime, error) {
	var out DateTime
	if err := c.getJSON(ctx, "/gopro/camera/get_date_time", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MediaList returns GET /gopro/media/list.
func (c *Client) MediaList(ctx context.Context) (*MediaList, error) {
	var out MediaList
	if err := c.getJSON(ctx, "/gopro/media/list", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MediaInfo returns GET /gopro/media/info?path=dir/name.
//
// The camera wants a literal "dir/name" in the query (an escaped "%2F" gives
// HTTP 400). GoPro filenames are plain ASCII, so raw concatenation is safe.
func (c *Client) MediaInfo(ctx context.Context, dir, name string) (*MediaInfo, error) {
	var out MediaInfo
	p := "/gopro/media/info?path=" + dir + "/" + name
	if err := c.getJSON(ctx, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadPath is the URL path for a stored media file.
func DownloadPath(dir, name string) string {
	return "/videos/DCIM/" + dir + "/" + name
}

// Head issues a HEAD for a media file. Returns (contentLength, statusCode).
// contentLength is -1 on any non-200.
func (c *Client) Head(ctx context.Context, dir, name string) (int64, int, error) {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(tctx, http.MethodHead, c.BaseURL+DownloadPath(dir, name), nil)
	if err != nil {
		return -1, 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return -1, 0, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1, resp.StatusCode, nil
	}
	return resp.ContentLength, resp.StatusCode, nil
}

// OpenFile starts a streaming GET for a media file. If rangeFrom > 0 a
// "Range: bytes=rangeFrom-" header is sent. The caller must Close the body.
// total is the full file size (already adjusted for a 206), -1 if unknown.
func (c *Client) OpenFile(ctx context.Context, dir, name string, rangeFrom int64) (body io.ReadCloser, total int64, resumed bool, err error) {
	var hdr http.Header
	if rangeFrom > 0 {
		hdr = http.Header{
			"Range":         {fmt.Sprintf("bytes=%d-", rangeFrom)},
			"Cache-Control": {"no-cache"},
		}
	}
	b, resp, err := c.get(ctx, DownloadPath(dir, name), hdr)
	if err != nil {
		return nil, 0, false, err
	}
	resumed = resp.StatusCode == http.StatusPartialContent

	// Trust the response only if it starts where we asked. This camera answers
	// an out-of-range request with 206 and a nonsensical Content-Range
	// ("bytes 2190774-2190772/2190773") instead of 416 -- measured on
	// MISSION 1 PRO ILS, 2026-09-05 -- and appending that to the .part would
	// corrupt it silently.
	if resumed && rangeFrom > 0 {
		start, ok := contentRangeStart(resp.Header.Get("Content-Range"))
		if !ok || start != rangeFrom {
			b.Close()
			return nil, 0, false, fmt.Errorf("%s: resume rejected: asked for byte %d, got Content-Range %q",
				name, rangeFrom, resp.Header.Get("Content-Range"))
		}
	}

	total = resp.ContentLength
	if resumed && rangeFrom > 0 && total >= 0 {
		total += rangeFrom
	}
	return b, total, resumed, nil
}

// contentRangeStart reads the first byte position out of a
// "bytes <start>-<end>/<total>" header.
func contentRangeStart(v string) (int64, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "bytes"))
	i := strings.Index(v, "-")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[:i]), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
