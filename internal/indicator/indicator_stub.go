//go:build !darwin || !cgo

package indicator

import "fmt"

func start() {}

func setIdle(string) {}

func setProgress(int, int, string) {}

func setMessage(string) {}

func WritePreview(string) error {
	return fmt.Errorf("indicator preview is only available on macOS with cgo")
}
