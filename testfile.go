package gotests

import "net/http"

type ifs interface {
	Action(str string) int64
}

type testStr struct {
	i     ifs
	ababa http.Hijacker
	a     string
}

func (t *testStr) Something(one, otherOne int64, two http.RoundTripper) (float64, error) {
	return 0, nil
}
