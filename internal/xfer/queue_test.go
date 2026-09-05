package xfer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/yager/gpget/internal/gopro"
)

// refusingTripper lets the first n RoundTrips through, then answers with
// ECONNREFUSED. That is what the next OpenFile looks like after the cable
// comes out: a new dial, not a mid-body EOF.
type refusingTripper struct {
	inner http.RoundTripper
	ok    int
	n     atomic.Int32
}

func (r *refusingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	i := r.n.Add(1)
	if int(i) <= r.ok {
		return r.inner.RoundTrip(req)
	}
	return nil, &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: syscall.ECONNREFUSED,
	}
}

func TestQueueStopsWhenCameraVanishes(t *testing.T) {
	body := []byte(strings.Repeat("x", 256))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	rt := &refusingTripper{inner: srv.Client().Transport, ok: 1}
	cl := gopro.NewClient(srv.URL)
	cl.HTTP.Transport = rt

	dir := t.TempDir()
	items := []Request{
		{Client: cl, Dir: "100GOPRO", Src: "A.JPG", FinalPath: filepath.Join(dir, "A.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "B.JPG", FinalPath: filepath.Join(dir, "B.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "C.JPG", FinalPath: filepath.Join(dir, "C.JPG"), ExpSize: int64(len(body))},
	}

	qr := ProcessItems(context.Background(), items, nil, nil)

	if qr.OK != 1 {
		t.Errorf("OK = %d, want 1", qr.OK)
	}
	if qr.Fail != 1 {
		t.Errorf("Fail = %d, want 1 (the unreachable item, not the ones after it)", qr.Fail)
	}
	if qr.Left != 1 {
		t.Errorf("Left = %d, want 1 (C must not be attempted)", qr.Left)
	}
	if qr.Unreachable == nil {
		t.Error("Unreachable is nil; the queue should have stopped on camera loss")
	}
	if got := rt.n.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (1 success + 1 refused; C must not be tried)", got)
	}

	if !exists(filepath.Join(dir, "A.JPG")) {
		t.Error("A.JPG was not finalized")
	}
	if exists(partPath(filepath.Join(dir, "B.JPG"))) {
		t.Error("0-byte .part left for B")
	}
	if exists(metaPath(filepath.Join(dir, "B.JPG"))) {
		t.Error(".part.meta left for B")
	}
	if exists(partPath(filepath.Join(dir, "C.JPG"))) {
		t.Error(".part created for C, which should not have been attempted")
	}
}

func TestQueueStopsOnCancel(t *testing.T) {
	body := []byte(strings.Repeat("z", 64))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cl := gopro.NewClient(srv.URL)
	dir := t.TempDir()
	items := []Request{
		{Client: cl, Dir: "100GOPRO", Src: "A.JPG", FinalPath: filepath.Join(dir, "A.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "B.JPG", FinalPath: filepath.Join(dir, "B.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "C.JPG", FinalPath: filepath.Join(dir, "C.JPG"), ExpSize: int64(len(body))},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	qr := ProcessItems(ctx, items, nil, nil)
	if qr.Left != 3 {
		t.Errorf("Left = %d, want 3 (nothing attempted)", qr.Left)
	}
	if qr.Unreachable != nil {
		t.Errorf("Unreachable = %v, want nil (this is cancel, not camera loss)", qr.Unreachable)
	}
	if !qr.Canceled {
		t.Error("Canceled is false; the queue should have recorded a cancel stop")
	}
	if qr.OK != 0 || qr.Fail != 0 {
		t.Errorf("OK=%d Fail=%d, want 0/0", qr.OK, qr.Fail)
	}
}

func TestQueueStopsOnDeadline(t *testing.T) {
	items := []Request{
		{Src: "A.JPG", FinalPath: "A.JPG"},
		{Src: "B.JPG", FinalPath: "B.JPG"},
		{Src: "C.JPG", FinalPath: "C.JPG"},
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	qr := ProcessItems(ctx, items, nil, nil)
	if qr.Unreachable != nil {
		t.Errorf("Unreachable = %v, want nil (deadline is not camera loss)", qr.Unreachable)
	}
	if !qr.TimedOut {
		t.Error("TimedOut is false")
	}
	if qr.Canceled {
		t.Error("Canceled is true; deadline should not look like Ctrl-C")
	}
	if qr.Left != 3 {
		t.Errorf("Left = %d, want 3", qr.Left)
	}
}

// A per-file HTTP error is not camera loss. The queue must continue.
func TestQueueContinuesAfterHTTPError(t *testing.T) {
	body := []byte(strings.Repeat("y", 64))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/B.JPG") {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cl := gopro.NewClient(srv.URL)
	dir := t.TempDir()
	items := []Request{
		{Client: cl, Dir: "100GOPRO", Src: "A.JPG", FinalPath: filepath.Join(dir, "A.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "B.JPG", FinalPath: filepath.Join(dir, "B.JPG"), ExpSize: int64(len(body))},
		{Client: cl, Dir: "100GOPRO", Src: "C.JPG", FinalPath: filepath.Join(dir, "C.JPG"), ExpSize: int64(len(body))},
	}

	qr := ProcessItems(context.Background(), items, nil, nil)
	if qr.OK != 2 || qr.Fail != 1 || qr.Left != 0 {
		t.Errorf("got OK=%d Fail=%d Left=%d, want OK=2 Fail=1 Left=0", qr.OK, qr.Fail, qr.Left)
	}
	if qr.Unreachable != nil {
		t.Errorf("Unreachable = %v, want nil", qr.Unreachable)
	}
	if !exists(filepath.Join(dir, "C.JPG")) {
		t.Error("C.JPG was not transferred after B's 404")
	}
}
