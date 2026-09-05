package gopro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// lossyString accepts a JSON string, number, bool, or null and keeps it as text.
// Integers are formatted in decimal so a size of 10737418240 does not become
// "1.073741824e+10" (which SizeBytes cannot parse).
type lossyString string

func (s *lossyString) UnmarshalJSON(data []byte) error {
	v, err := decodeLossyString(data)
	if err != nil {
		return err
	}
	*s = lossyString(v)
	return nil
}

type lossyStringSlice []string

func (s *lossyStringSlice) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = nil
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make([]string, len(raw))
	for i, r := range raw {
		v, err := decodeLossyString(r)
		if err != nil {
			return err
		}
		out[i] = v
	}
	*s = out
	return nil
}

func decodeLossyString(data []byte) (string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return "", nil
	}
	switch data[0] {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return "", err
		}
		return s, nil
	case 't':
		if bytes.Equal(data, []byte("true")) {
			return "true", nil
		}
	case 'f':
		if bytes.Equal(data, []byte("false")) {
			return "false", nil
		}
	}
	// json.Number is a string alias, so json.Unmarshal cannot decode a number
	// token into it. The token bytes are already the decimal form we want.
	if data[0] == '-' || data[0] == '+' || (data[0] >= '0' && data[0] <= '9') {
		n := json.Number(data)
		if i, err := n.Int64(); err == nil {
			return strconv.FormatInt(i, 10), nil
		}
		return string(n), nil
	}
	return "", fmt.Errorf("not a string, number, or bool: %s", data)
}

// UnmarshalJSON keeps MediaItem's exported fields as string / []string so
// callers do not change, while still accepting values the camera may send as
// numbers or booleans instead of strings.
func (m *MediaItem) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name    lossyString      `json:"n"`
		Size    lossyString      `json:"s"`
		Cre     lossyString      `json:"cre"`
		Mod     lossyString      `json:"mod"`
		Raw     lossyString      `json:"raw"`
		GLRV    lossyString      `json:"glrv"`
		LS      lossyString      `json:"ls"`
		Group   lossyString      `json:"g"`
		Begin   lossyString      `json:"b"`
		Last    lossyString      `json:"l"`
		TypeTag lossyString      `json:"t"`
		Missing lossyStringSlice `json:"m"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Name = string(aux.Name)
	m.Size = string(aux.Size)
	m.Cre = string(aux.Cre)
	m.Mod = string(aux.Mod)
	m.Raw = string(aux.Raw)
	m.GLRV = string(aux.GLRV)
	m.LS = string(aux.LS)
	m.Group = string(aux.Group)
	m.Begin = string(aux.Begin)
	m.Last = string(aux.Last)
	m.TypeTag = string(aux.TypeTag)
	m.Missing = []string(aux.Missing)
	return nil
}

func (mi *MediaInfo) UnmarshalJSON(data []byte) error {
	var aux struct {
		Cre lossyString `json:"cre"`
		Siz lossyString `json:"s"`
		CT  lossyString `json:"ct"`
		W   lossyString `json:"w"`
		H   lossyString `json:"h"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	mi.Cre = string(aux.Cre)
	mi.Siz = string(aux.Siz)
	mi.CT = string(aux.CT)
	mi.W = string(aux.W)
	mi.H = string(aux.H)
	return nil
}
