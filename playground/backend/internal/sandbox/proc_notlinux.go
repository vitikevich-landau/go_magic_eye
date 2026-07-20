//go:build !linux

package sandbox

// Вне Linux prlimit на чужой процесс недоступен — честная деградация:
// потолок памяти обеспечивает контейнер (mem_limit в compose), а
// GOMEMLIMIT остаётся мягким ориентиром для GC.
func applyMemLimit(int, int64) error { return nil }
