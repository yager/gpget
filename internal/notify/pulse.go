package notify

import "time"

// TransferID is the replace-key for a live autostart transfer. Start, progress
// pulses, and the final 完了/失敗 all share it so the banner stays one card.
const TransferID = "gpget-xfer"

const (
	PulseEveryFiles = 10
	PulseEvery      = 15 * time.Second
)

// ShouldPulse reports whether a progress banner should be refreshed.
// The opening 0/N card and the final 完了/失敗 are sent by the caller;
// this only covers the in-between updates.
func ShouldPulse(done, total, lastDone int, since time.Duration) bool {
	if done <= 0 || total <= 0 || done >= total {
		return false
	}
	if done-lastDone >= PulseEveryFiles {
		return true
	}
	return since >= PulseEvery
}

// Pulser throttles progress notifications. Nil-safe.
type Pulser struct {
	id    string
	total int
	lastN int
	lastT time.Time
}

func NewPulser(id string, total int) *Pulser {
	if id == "" {
		id = TransferID
	}
	return &Pulser{id: id, total: total, lastT: time.Now()}
}

func (p *Pulser) File(done int, title, body string) {
	if p == nil {
		return
	}
	if !ShouldPulse(done, p.total, p.lastN, time.Since(p.lastT)) {
		return
	}
	Pulse(p.id, title, body)
	p.lastN = done
	p.lastT = time.Now()
}
