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

// Code returns the stable error_code carried by err ("" if none).
func Code(err error) string {
	if err == nil {
		return ""
	}
	if c, ok := err.(coded); ok {
		return c.code
	}
	return ""
}
