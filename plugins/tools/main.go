package main

func main() {
	p := createPlugin()
	p.sdk.Register("tool", "list", p.handleListTools)

}
