package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/pipeline"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// URLGetter is implemented by objects that can perform HTTP GET requests.
type URLGetter interface {
	Get(url string) (*http.Response, error)
}

// PrivateNetworkDetector is implemented by objects that can detect whether a
// host resolves to a private network address.
type PrivateNetworkDetector interface {
	IsPrivate(host string) (bool, error)
}

type linkFetcher struct {
	urlGetter   URLGetter
	netDetector PrivateNetworkDetector
}

func newLinkFetcher(urlGetter URLGetter, netDetector PrivateNetworkDetector) *linkFetcher {
	return &linkFetcher{
		urlGetter:   urlGetter,
		netDetector: netDetector,
	}
}

func (lf *linkFetcher) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*crawlerPayload)

	// Skip URLs that point to files that cannot contain html content.
	if exclusionRegex.MatchString(payload.URL) {
		return nil, nil
	}

	// Never crawl links in private networks (e.g. link-local addresses).
	// This is a security risk!
	if isPrivate, err := lf.isPrivate(payload.URL); err != nil || isPrivate {
		return nil, nil
	}

	// If the link is good, then initiate the request.
	res, err := lf.urlGetter.Get(payload.URL)
	if err != nil {
		return nil, nil
	}

	// Skip payloads for invalid http status codes.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		// If the status code is non-2XX, then we will drop the payload altogether.
		return nil, nil
	}

	// Skip payloads for non-html payloads.
	if contentType := res.Header.Get("Content-Type"); !strings.Contains(contentType, "html") {
		return nil, nil
	}

	// Otherwise, copy over the response to the payload.
	_, err = io.Copy(&payload.RawContent, res.Body)
	_ = res.Body.Close()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (lf *linkFetcher) isPrivate(URL string) (bool, error) {
	u, err := url.Parse(URL)
	if err != nil {
		return false, err
	}
	return lf.netDetector.IsPrivate(u.Hostname())
}
