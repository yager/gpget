package gopro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const mediaItemStringJSON = `{
  "n": "GPAD0099.JPG",
  "s": "17191943",
  "cre": "1788540010",
  "mod": "1788540010",
  "raw": "1",
  "g": "1004",
  "b": "99",
  "l": "102",
  "t": "b",
  "m": ["100"]
}`

const mediaItemNumberJSON = `{
  "n": "GPAD0099.JPG",
  "s": 17191943,
  "cre": 1788540010,
  "mod": 1788540010,
  "raw": 1,
  "g": 1004,
  "b": 99,
  "l": 102,
  "t": "b",
  "m": [100]
}`

func TestMediaItemAcceptsStringOrNumberJSON(t *testing.T) {
	var asString, asNumber MediaItem
	if err := json.Unmarshal([]byte(mediaItemStringJSON), &asString); err != nil {
		t.Fatalf("string JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(mediaItemNumberJSON), &asNumber); err != nil {
		t.Fatalf("number JSON: %v", err)
	}
	if !reflect.DeepEqual(asString, asNumber) {
		t.Errorf("string vs number:\n  %#v\n  %#v", asString, asNumber)
	}
	if asString.Name != "GPAD0099.JPG" {
		t.Errorf("Name = %q", asString.Name)
	}
	if asString.SizeBytes() != 17191943 {
		t.Errorf("SizeBytes = %d", asString.SizeBytes())
	}
	if asString.CreUnix() != 1788540010 {
		t.Errorf("CreUnix = %d", asString.CreUnix())
	}
	if got := asString.Missing; len(got) != 1 || got[0] != "100" {
		t.Errorf("Missing = %#v, want [\"100\"]", got)
	}
}

func TestMediaItemNumberDoesNotUseScientificNotation(t *testing.T) {
	var m MediaItem
	if err := json.Unmarshal([]byte(`{"n":"GX010014.MP4","s":10737418240}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Size != "10737418240" {
		t.Errorf("Size = %q, want the decimal integer (not scientific notation)", m.Size)
	}
	if m.SizeBytes() != 10737418240 {
		t.Errorf("SizeBytes = %d", m.SizeBytes())
	}
}

func TestMediaListDecodesILSCapture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "media-list-ils.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ml MediaList
	if err := json.Unmarshal(raw, &ml); err != nil {
		t.Fatal(err)
	}
	if len(ml.Media) != 1 {
		t.Fatalf("dirs = %d, want 1", len(ml.Media))
	}
	fs := ml.Media[0].FS
	if len(fs) != 16 {
		t.Fatalf("entries = %d, want 16", len(fs))
	}
	var groups int
	var missing *MediaItem
	for i := range fs {
		it := &fs[i]
		if it.IsGroup() {
			groups++
		}
		if it.Name == "GPAD0099.JPG" {
			missing = it
		}
	}
	if groups != 4 {
		t.Errorf("groups = %d, want 4", groups)
	}
	if missing == nil {
		t.Fatal("GPAD0099.JPG not in the capture")
	}
	if got := missing.Missing; len(got) != 1 || got[0] != "100" {
		t.Errorf("GPAD0099 Missing = %#v, want [\"100\"]", missing.Missing)
	}
	if missing.SizeBytes() != 17191943 {
		t.Errorf("GPAD0099 SizeBytes = %d", missing.SizeBytes())
	}
}

func TestMediaInfoAcceptsNumberCT(t *testing.T) {
	var asString, asNumber MediaInfo
	if err := json.Unmarshal([]byte(`{"cre":"1","s":"2","ct":"5","w":"4000","h":"3000"}`), &asString); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"cre":1,"s":2,"ct":5,"w":4000,"h":3000}`), &asNumber); err != nil {
		t.Fatal(err)
	}
	if asString != asNumber {
		t.Errorf("string vs number: %#v vs %#v", asString, asNumber)
	}
	if asNumber.CTInt() != 5 {
		t.Errorf("CTInt = %d", asNumber.CTInt())
	}
}
