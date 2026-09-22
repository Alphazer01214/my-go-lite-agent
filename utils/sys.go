package utils

import "runtime"

func IsLinux() bool {
	return runtime.GOOS == "linux"
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}
