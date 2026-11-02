//go:build !darwin && !windows

package resources

import "errors"

type nativeProvider struct{}

func Native() Provider            { return nativeProvider{} }
func (nativeProvider) Cores() int { return 1 }
func (nativeProvider) List() ([]Process, bool, error) {
	return nil, false, errors.New("resource measurement unavailable on this platform")
}
func (nativeProvider) Read(Process) (Reading, error) { return Reading{}, errors.New("unavailable") }
