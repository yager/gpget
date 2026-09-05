package gopro

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
)

// Candidate is a network interface that might reach a GoPro.
type Candidate struct {
	Iface    string // e.g. "en10"
	HostIP   net.IP // this machine's address on that subnet
	CameraIP net.IP // derived camera address (host subnet, .51)
}

// Found is a confirmed GoPro reachable on the network.
type Found struct {
	Candidate
	Info        *CameraInfo
	SerialMatch bool // camera IP matches the serial-derived 172.2X.1YZ.51 formula
}

// enumerateCandidates returns interfaces with a private 172.16-31.x.x/24 address.
// Docker bridges (docker0 / br-*) and VPNs can also land in this range, so every
// candidate must be probed before use.
func enumerateCandidates() ([]Candidate, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || !isGoProRange(ip4) {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones != 24 {
				continue
			}
			cam := make(net.IP, 4)
			copy(cam, ip4)
			cam[3] = 51
			out = append(out, Candidate{Iface: ifc.Name, HostIP: ip4, CameraIP: cam})
		}
	}
	return out, nil
}

// PresentInterfaces lists GoPro-shaped interfaces (a private 172.16-31.x.x/24
// address on an up, non-loopback interface) WITHOUT touching the network. A
// background trigger that must not make blocking connections — e.g. a macOS
// LaunchAgent, which cannot reach local-network addresses — uses this to notice
// that a camera was plugged in, then hands the actual HTTP work to a process
// that does have local-network access.
func PresentInterfaces() []Candidate {
	c, _ := enumerateCandidates()
	return c
}

// isGoProRange reports whether ip is in 172.16.0.0 – 172.31.255.255.
func isGoProRange(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31
}

// Discover finds GoPro cameras on the local network. It enumerates candidate
// interfaces, probes each candidate's .51 with GET /gopro/camera/info, and
// returns only the ones that answered like a GoPro.
func Discover(ctx context.Context) ([]Found, error) {
	cands, err := enumerateCandidates()
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, nil
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found []Found
	)
	for _, cand := range cands {
		cand := cand
		wg.Add(1)
		go func() {
			defer wg.Done()
			cl := NewClient(cand.CameraIP.String())
			info, err := cl.Info(ctx)
			if err != nil || info == nil || info.ModelName == "" {
				return
			}
			f := Found{
				Candidate:   cand,
				Info:        info,
				SerialMatch: serialMatchesIP(info.SerialNumber, cand.CameraIP),
			}
			mu.Lock()
			found = append(found, f)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return found, nil
}

// serialMatchesIP checks the documented wired-IP formula: the last 3 digits XYZ
// of the serial map to 172.2X.1YZ.51.
func serialMatchesIP(serial string, ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil || len(serial) < 3 {
		return false
	}
	tail := serial[len(serial)-3:]
	for _, r := range tail {
		if r < '0' || r > '9' {
			return false
		}
	}
	want := fmt.Sprintf("172.2%c.1%c%c.51", tail[0], tail[1], tail[2])
	return ip4.String() == want
}

// ResolveBaseURL decides which camera URL to use.
//   - explicit != ""  -> use it verbatim (still normalized by NewClient)
//   - otherwise Discover; exactly one hit -> use it; zero -> error; many -> error
//     listing them so the user can pass --ip.
func ResolveBaseURL(ctx context.Context, explicit string) (string, *Found, error) {
	if strings.TrimSpace(explicit) != "" {
		return normalizeBase(explicit), nil, nil
	}
	found, err := Discover(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("interface scan failed: %w", err)
	}
	switch len(found) {
	case 0:
		return "", nil, fmt.Errorf("no GoPro found on any 172.16-31.x.x interface — connect the camera by USB, or pass --ip")
	case 1:
		f := found[0]
		return normalizeBase(f.CameraIP.String()), &f, nil
	default:
		var b strings.Builder
		b.WriteString("multiple GoPro-looking cameras found; pass --ip <addr>:\n")
		for _, f := range found {
			fmt.Fprintf(&b, "  %s  (%s, %s)\n", f.CameraIP, f.Info.ModelName, f.Iface)
		}
		return "", nil, fmt.Errorf("%s", b.String())
	}
}
