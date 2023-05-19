package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/pipeline"
	"net/url"
	"regexp"
	"strings"
)

var (
	// Skip any URLs that do not point to HTML content.
	exclusionRegex = regexp.MustCompile(`(?i)\.(?:jpg|jpeg|png|gif|ico|css|js)$`)

	// Find any <base href="XXX"> tags.
	baseHrefRegex = regexp.MustCompile(`(?i)<base.*?href\s*?=\s*?"(.*?)\s*?"`)

	// Extract the link content from <a href="XXX"> tags.
	findLinkRegex = regexp.MustCompile(`(?i)<a.*?href\s*?=\s*?"\s*?(.*?)\s*?".*?>`)

	// Determine if the rel="nofollow" attribute is set.
	nofollowRegex = regexp.MustCompile(`(?i)rel\s*?=\s*?"?nofollow"?`)
)

type linkExtractor struct {
	netDetector PrivateNetworkDetector
}

func newLinkExtractor(netDetector PrivateNetworkDetector) *linkExtractor {
	return &linkExtractor{
		netDetector: netDetector,
	}
}

func (le *linkExtractor) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*crawlerPayload)
	relTo, err := url.Parse(payload.URL)
	if err != nil {
		return nil, err
	}
	content := payload.RawContent.String()

	// Search page content for a <base> tag and resolve it to an absolute URL.
	if baseMatch := baseHrefRegex.FindStringSubmatch(content); len(baseMatch) == 2 {
		if base := resolveURL(relTo, ensureHasTrailingSlash(baseMatch[1])); base != nil {
			relTo = base
		}
	}

	// Find the unique set of links from the document, resolve them and
	// add them to the payload.
	seen := make(map[string]struct{})
	for _, match := range findLinkRegex.FindAllStringSubmatch(content, -1) {
		// Create an absolute URL from each <a> tag.
		link := resolveURL(relTo, match[1])

		// If this link isn't safe to process, then drop it.
		if !le.retainLink(relTo.Hostname(), link) {
			continue
		}

		// Truncate anchors from the end of the link. (eg. example.com/#fragment)
		link.Fragment = ""

		// Drop any duplicate links.
		linkStr := link.String()
		if _, ok := seen[linkStr]; ok {
			continue
		}

		// Skip URLs that point to files that cannot contain html content.
		if exclusionRegex.MatchString(linkStr) {
			continue
		}

		// Otherwise, mark this string as processed.
		seen[linkStr] = struct{}{}
		if nofollowRegex.MatchString(match[0]) {
			payload.NoFollowLinks = append(payload.NoFollowLinks, linkStr)
		} else {
			payload.Links = append(payload.Links, linkStr)
		}
	}

	return payload, nil
}

// retainLink determines if a link is ok to consider processing.
func (le *linkExtractor) retainLink(srcHost string, link *url.URL) bool {
	// Skip links that could not be resolved.
	if link == nil {
		return false
	}

	// Skip links with non http(s) schemes.
	if link.Scheme != "http" && link.Scheme != "https" {
		return false
	}

	// Keep links to the same host.
	// We've already vetted this host in the previous stage (the link fetcher),
	// so if the host hasn't changed, we don't need to check it again.
	// This is just a small optimization, and can be omitted.
	if link.Hostname() == srcHost {
		return true
	}

	// Skip links that resolve to private networks.
	if isPrivate, err := le.netDetector.IsPrivate(link.Host); err != nil || isPrivate {
		return false
	}

	return true
}

func ensureHasTrailingSlash(s string) string {
	if s[len(s)-1] != '/' {
		return s + "/"
	}
	return s
}

// resolveURL expands target into an absolute URL using the following rules:
//   - targets starting with '//' are treated as absolute URLs that inherit the
//     protocol from relTo.
//   - targets starting with '/' are absolute URLs that are appended to the host
//     from relTo.
//   - all other targets are assumed to be relative to relTo.
//
// If the target URL cannot be parsed, a nil URL wil be returned.
func resolveURL(relTo *url.URL, target string) *url.URL {
	tLen := len(target)
	if tLen == 0 {
		return nil
	}

	if strings.HasPrefix(target, "//") {
		target = relTo.Scheme + ":" + target
	}

	if targetURL, err := url.Parse(target); err == nil {
		return relTo.ResolveReference(targetURL)
	}

	return nil
}
