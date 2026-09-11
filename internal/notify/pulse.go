package notify

import "time"

// TransferID is the replace-key historically used for a live autostart
// transfer banner. Progress now goes to the menu-bar indicator; complete/fail
// use Send. Kept so older call sites and tests still compile.
const TransferID = "gpget-xfer"

const (
	PulseEveryFiles = 10
	PulseEvery      = 15 * time.Second
)

// ShouldPulse reports whether a progress banner should be refreshed.
// Autostart no longer pulses; retained for unit tests of the old throttle.
func ShouldPulse(done, total, lastDone int, since time.Duration) bool {
	if done <= 0 || total <= 0 || done >= total {
		return false
	}
	if done-lastDone >= PulseEveryFiles {
		return true
	}
	return since >= PulseEvery
}

// Pulser throttles progress notifications. Nil-safe. Unused by autostart after
// the menu-bar indicator landed; kept for callers that still want banners.
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
