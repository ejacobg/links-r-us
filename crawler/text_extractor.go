package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/pipeline"
	"github.com/microcosm-cc/bluemonday"
	"html"
	"regexp"
	"strings"
	"sync"
)

var (
	titleRegex         = regexp.MustCompile(`(?i)<title.*?>(.*?)</title>`)
	repeatedSpaceRegex = regexp.MustCompile(`\s+`)
)

type textExtractor struct {
	// bluemonday policies are not thread-safe, so we need to create a new one every time we want to use it.
	policyPool sync.Pool
}

func newTextExtractor() *textExtractor {
	return &textExtractor{
		policyPool: sync.Pool{
			New: func() interface{} {
				return bluemonday.StrictPolicy()
			},
		},
	}
}

func (te *textExtractor) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*crawlerPayload)
	policy := te.policyPool.Get().(*bluemonday.Policy)

	// If we find a <title> tag, use the policy to extract the text content from it.
	if titleMatch := titleRegex.FindStringSubmatch(payload.RawContent.String()); len(titleMatch) == 2 {
		payload.Title = strings.TrimSpace(html.UnescapeString(repeatedSpaceRegex.ReplaceAllString(
			policy.Sanitize(titleMatch[1]), " ",
		)))
	}

	// Reuse the policy to strip the HTML tags from the rest of the content.
	payload.TextContent = strings.TrimSpace(html.UnescapeString(repeatedSpaceRegex.ReplaceAllString(
		policy.SanitizeReader(&payload.RawContent).String(), " ",
	)))
	te.policyPool.Put(policy)

	return payload, nil
}
