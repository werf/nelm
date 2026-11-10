package main

import (
	"io"
	"net/http"
	"strings"
)

var _ http.RoundTripper = (fakeUpstreamTransport)(nil)

type fakeUpstreamTransport map[string]string

func (f fakeUpstreamTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, ok := f[request.URL.String()]
	if !ok {
		return &http.Response{
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
			Status:     "404 Not Found",
			StatusCode: http.StatusNotFound,
		}, nil
	}

	return &http.Response{
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
		Status:     "200 OK",
		StatusCode: http.StatusOK,
	}, nil
}
