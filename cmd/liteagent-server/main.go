package main

import "os"

// version 由 scripts/build.sh 和 scripts/build.ps1 通过
// -ldflags "-X main.version=..." 注入。直接 go build 时保持 dev。
var version = "dev"

func main() {
	os.Exit(run())
}
