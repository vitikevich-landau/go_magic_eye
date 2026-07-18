//go:build !linux && !darwin && !windows

package term

import "errors"

// Прочие ОС: Око честно признаётся, что интерактива нет, и печатает статикой.

func isTerminal(fd uintptr) bool       { return false }
func size(fd uintptr) (int, int, bool) { return 0, 0, false }
func enableColor(fd uintptr) bool      { return false }
func raw() (func(), error) {
	return nil, errors.New("eye: на этой платформе raw-терминал не поддержан — только статическая печать")
}
func readInput(p []byte, timeoutMS int) (int, error) {
	return 0, errors.New("eye: нет raw-терминала")
}
