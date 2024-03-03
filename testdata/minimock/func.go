package minimock

import (
	"context"
	"io"
	"net/http"
)

type inputIf interface {
	Do(string) (int, error)
}

type outIf interface {
	Out(float64) error
}

type someStruct struct {
	firstVal  http.RoundTripper
	secondVal string
	wr        io.Writer
	ctx       context.Context
}

func (s *someStruct) someFunc(ctx context.Context, i inputIf, f string, e io.Writer) (outIf, int, error) {
	return nil, 0, nil
}

type b string

func (r b) Do(i inputIf) outIf {
	return nil
}

func g(a string) string {
	return ""
}
