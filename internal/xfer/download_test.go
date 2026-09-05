package xfer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yager/gpget/internal/gopro"
)

// ---- helpers ---------------------------------------------------------------

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stageComplete writes a .part holding every byte of body, plus a matching meta.
func stageComplete(t *testing.T, final string, body []byte) {
	t.Helper()
	writeFile(t, partPath(final), body)
	if err := WriteMeta(final, PartMeta{
		Src:     "100GOPRO/" + filepath.Base(final),
		ExpSize: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
}

func baseRequest(final string, body []byte) Request {
	return Request{
		Dir:       "100GOPRO",
		Src:       filepath.Base(final),
		FinalPath: final,
		ExpSize:   int64(len(body)),
		Overwrite: "skip",
	}
}

// ---- #1: resume decision ---------------------------------------------------

// A .part that already holds every expected byte must be finalized without
// contacting the camera. Asking for a Range at EOF is out of range, and this
// camera answers with 206 plus zero bytes rather than 416.
func TestCompletePartFinalizesWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	body := []byte("0123456789abcdef")
	stageComplete(t, final, body)

	// A nil Client makes any network access a panic, which is the point.
	res, err := Download(context.Background(), baseRequest(final, body))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Skipped {
		t.Error("reported as skipped; the .part should have been finalized")
	}
	if got := read(t, final); string(got) != string(body) {
		t.Errorf("content = %q, want %q", got, body)
	}
	if exists(partPath(final)) {
		t.Error(".part still present after finalize")
	}
	if exists(metaPath(final)) {
		t.Error(".part.meta still present after finalize")
	}
}

// A .part longer than the camera says the file is cannot be a prefix of it, so
// appending would only make it worse. Fail with something the user can act on,
// and keep the file for them to inspect.
func TestOversizedPartIsRejectedAndKept(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	body := []byte("0123456789abcdef")

	writeFile(t, partPath(final), append(append([]byte{}, body...), []byte("EXTRA")...))
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/GP010009.JPG", ExpSize: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	_, err := Download(context.Background(), baseRequest(final, body))
	if err == nil {
		t.Fatal("expected an error for an oversized .part")
	}
	if !strings.Contains(err.Error(), "larger than expected") {
		t.Errorf("error = %v, want it to say the .part is larger than expected", err)
	}
	if !exists(partPath(final)) {
		t.Error(".part was deleted; it should be kept for the user to inspect")
	}
}

func TestStalePartIsReported(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	body := []byte("0123456789abcdef")

	writeFile(t, partPath(final), body[:4])
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/SOMETHING_ELSE.JPG", ExpSize: 999}); err != nil {
		t.Fatal(err)
	}

	_, err := Download(context.Background(), baseRequest(final, body))
	if !errors.Is(err, ErrStalePart) {
		t.Errorf("err = %v, want ErrStalePart", err)
	}
}

// ---- #2: finalize ----------------------------------------------------------

func TestFinalizeReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, final, []byte("old"))
	writeFile(t, partPath(final), []byte("new"))

	if err := finalize(partPath(final), final); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := read(t, final); string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
	if exists(partPath(final)) {
		t.Error(".part still present")
	}
}

// The destination must never be destroyed on a failed finalize. Removing it
// before the rename -- or as a blind fallback -- would leave the user with
// neither the old file nor the new one.
func TestFinalizeKeepsDestinationWhenPartIsMissing(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, final, []byte("precious"))

	if err := finalize(partPath(final), final); err == nil {
		t.Fatal("expected an error when the .part is missing")
	}
	if !exists(final) {
		t.Fatal("destination was deleted on a failed finalize")
	}
	if got := read(t, final); string(got) != "precious" {
		t.Errorf("content = %q, want it untouched", got)
	}
}

func TestFinalizeIntoEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, partPath(final), []byte("new"))

	if err := finalize(partPath(final), final); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := read(t, final); string(got) != "new" {
		t.Errorf("content = %q", got)
	}
}

// ---- #3: interrupted after the last byte ------------------------------------

func TestExpectedBytesPrefersListedSize(t *testing.T) {
	for _, c := range []struct{ listed, contentLength, want int64 }{
		{100, 200, 100},
		{-1, 200, 200},
		{-1, -1, -1},
		{0, 50, 0},
	} {
		if got := expectedBytes(c.listed, c.contentLength); got != c.want {
			t.Errorf("expectedBytes(%d, %d) = %d, want %d", c.listed, c.contentLength, got, c.want)
		}
	}
}

// truncatingServer sends `declared` as Content-Length but only `send` bytes,
// then drops the connection. That is what a cut cable looks like to the client.
func truncatingServer(t *testing.T, body []byte, declared int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter is not a Hijacker")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", declared)
		buf.Write(body)
		buf.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The connection can break after the last byte arrives. Every expected byte is
// on disk, so the transfer succeeded -- failing it would strand a complete
// .part that the next run cannot resume (a Range at EOF is out of range).
func TestInterruptionAfterLastByteSucceeds(t *testing.T) {
	body := []byte(strings.Repeat("x", 4096))
	srv := truncatingServer(t, body, len(body)+512) // claims more than it sends

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(srv.URL)

	res, err := Download(context.Background(), r)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Bytes != int64(len(body)) {
		t.Errorf("Bytes = %d, want %d", res.Bytes, len(body))
	}
	if got := read(t, final); len(got) != len(body) {
		t.Errorf("final size = %d, want %d", len(got), len(body))
	}
	if exists(partPath(final)) {
		t.Error(".part still present after a successful transfer")
	}
}

// A short transfer must still fail, or the guard above would mask real losses.
func TestShortTransferStillFails(t *testing.T) {
	body := []byte(strings.Repeat("x", 4096))
	srv := truncatingServer(t, body[:1000], len(body)) // really is short

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(srv.URL)

	if _, err := Download(context.Background(), r); err == nil {
		t.Fatal("expected an error for a genuinely short transfer")
	}
	if exists(final) {
		t.Error("final file was created from an incomplete transfer")
	}
}

// ---- resume against a server -------------------------------------------------

// rangeServer serves body, honouring Range when honour is true. When it is
// false it answers a Range request with the whole file and a plain 200, which
// is legal HTTP and would corrupt the .part if appended.
func rangeServer(t *testing.T, body []byte, honour bool, forceStart int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" || !honour {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.WriteHeader(http.StatusOK)
			w.Write(body)
			return
		}
		var start int64
		fmt.Sscanf(rng, "bytes=%d-", &start)
		reported := start
		if forceStart >= 0 {
			reported = forceStart // mis-stated Content-Range, as this camera does
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", reported, len(body)-1, len(body)))
		w.Header().Set("Content-Length", fmt.Sprint(int64(len(body))-start))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[start:])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResumeContinuesFromPart(t *testing.T) {
	body := []byte(strings.Repeat("abcd", 1024))
	srv := rangeServer(t, body, true, -1)

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, partPath(final), body[:1000])
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/GP010009.JPG", ExpSize: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(srv.URL)

	res, err := Download(context.Background(), r)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.Resumed {
		t.Error("Resumed = false, want true")
	}
	if got := read(t, final); string(got) != string(body) {
		t.Errorf("content mismatch: got %d bytes, want %d", len(got), len(body))
	}
}

// This camera answers an out-of-range request with 206 and a Content-Range that
// does not start where we asked. Appending that would corrupt the .part.
func TestResumeRejectsMisstatedContentRange(t *testing.T) {
	body := []byte(strings.Repeat("abcd", 1024))
	srv := rangeServer(t, body, true, 0) // always claims it started at 0

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, partPath(final), body[:1000])
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/GP010009.JPG", ExpSize: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(srv.URL)

	_, err := Download(context.Background(), r)
	if err == nil {
		t.Fatal("expected the resume to be rejected")
	}
	if !strings.Contains(err.Error(), "resume rejected") {
		t.Errorf("error = %v, want it to name the rejected resume", err)
	}
	if exists(final) {
		t.Error("a final file was produced from a rejected resume")
	}
}

// A camera that cannot be reached at all must not leave a 0-byte .part and
// .part.meta behind. Those files are for resume, and there is nothing to resume.
func TestDownloadCleansEmptyPartWhenCameraUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	r := baseRequest(final, []byte("0123456789abcdef"))
	r.Client = gopro.NewClient(url)

	_, err := Download(context.Background(), r)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if exists(partPath(final)) {
		t.Error("0-byte .part was left behind")
	}
	if exists(metaPath(final)) {
		t.Error(".part.meta was left behind")
	}
}

// A .part that already holds some bytes is the resume prefix. Camera loss
// must not delete it.
func TestDownloadKeepsPartialPartWhenCameraUnreachable(t *testing.T) {
	body := []byte(strings.Repeat("x", 4096))
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, partPath(final), body[:1000])
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/GP010009.JPG", ExpSize: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(url)
	if _, err := Download(context.Background(), r); err == nil {
		t.Fatal("expected a connection error")
	}
	if !exists(partPath(final)) {
		t.Error("partial .part was deleted; it should be kept for resume")
	}
}

// A 200 answer to a Range request means the whole file is coming. Appending it
// to the .part would corrupt the result, so the transfer must restart at zero.
func TestRangeIgnoredRestartsFromZero(t *testing.T) {
	body := []byte(strings.Repeat("abcd", 1024))
	srv := rangeServer(t, body, false, -1)

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, partPath(final), body[:1000])
	if err := WriteMeta(final, PartMeta{Src: "100GOPRO/GP010009.JPG", ExpSize: int64(len(body))}); err != nil {
		t.Fatal(err)
	}

	r := baseRequest(final, body)
	r.Client = gopro.NewClient(srv.URL)

	if _, err := Download(context.Background(), r); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := read(t, final); string(got) != string(body) {
		t.Errorf("content mismatch: got %d bytes, want %d — the .part was appended to instead of restarted",
			len(got), len(body))
	}
}

func TestSkipDoesNotKeepWrongSizeFile(t *testing.T) {
	body := []byte("the-real-bytes")
	srv := rangeServer(t, body, true, -1)

	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.JPG")
	writeFile(t, final, []byte("short"))

	r := baseRequest(final, body)
	r.Overwrite = "skip"
	r.Client = gopro.NewClient(srv.URL)

	res, err := Download(context.Background(), r)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Skipped {
		t.Fatal("skip kept a short file; F-5 requires a size match")
	}
	if got := read(t, final); string(got) != string(body) {
		t.Errorf("content = %q, want %q", got, body)
	}
}

func TestSkipKeepsUnknownSizeWhenPresent(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "GP010009.GPR")
	writeFile(t, final, []byte("whatever"))

	r := Request{FinalPath: final, ExpSize: -1, Overwrite: "skip", Src: "GP010009.GPR"}
	res, err := Download(context.Background(), r)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.Skipped {
		t.Fatal("unknown-size file that exists should still skip")
	}
}
