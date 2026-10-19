package main

// errCoded builds a wire-coded error (pluginsdk.ErrCode compatible shape).
// Defined here so store.go stays free of a pluginsdk import cycle in tests.
func errCoded(code, msg string) error {
	return coded{code: code, msg: msg}
}

type coded struct {
	code string
	msg  string
}

func (e coded) Error() string { return e.msg }
