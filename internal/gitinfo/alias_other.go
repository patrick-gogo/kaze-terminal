//go:build !windows

package gitinfo

func storedAlias(dir, part string) (string, bool) { return "", false }
