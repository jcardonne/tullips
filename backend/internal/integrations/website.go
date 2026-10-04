package integrations

import (
	"context"
	"errors"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// PublicIP rejects local, private and special-use destinations, including mapped IPv4.
func PublicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return false
		}
	}
	return true
}
func publicURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "80" && u.Port() != "443") {
		return nil, errors.New("expected public HTTP(S) URL")
	}
	return u, nil
}
func WebsiteClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("host has no address")
		}
		for _, ip := range ips {
			if !PublicIP(ip.IP) {
				return nil, errors.New("private destination rejected")
			}
		} // Pin the validated address to prevent DNS rebinding.
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		_, e := publicURL(req.URL.String())
		return e
	}}
}

type Page struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Crawl follows a bounded set of same-origin pages. Page content is untrusted AI input.
func Crawl(ctx context.Context, raw string, maxPages int) ([]Page, error) {
	base, e := publicURL(raw)
	if e != nil {
		return nil, e
	}
	if maxPages < 1 || maxPages > 8 {
		maxPages = 5
	}
	client := WebsiteClient()
	queue := []string{base.String()}
	seen := map[string]bool{}
	pages := []Page{}
	for len(queue) > 0 && len(pages) < maxPages {
		next := queue[0]
		queue = queue[1:]
		if seen[next] {
			continue
		}
		seen[next] = true
		req, e := http.NewRequestWithContext(ctx, "GET", next, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("User-Agent", "Tullips/1.0 website analysis")
		resp, e := client.Do(req)
		if e != nil {
			if len(pages) == 0 {
				return nil, e
			}
			continue
		}
		b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if e != nil {
			return nil, e
		}
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			if len(pages) == 0 {
				return nil, errors.New("website did not return HTML")
			}
			continue
		}
		doc, e := html.Parse(strings.NewReader(string(b)))
		if e != nil {
			return nil, e
		}
		var text strings.Builder
		title := ""
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
				return
			}
			if n.Type == html.TextNode {
				if n.Parent != nil && n.Parent.Data == "title" {
					title += n.Data
				}
				if text.Len() < 50000 {
					text.WriteString(n.Data)
					text.WriteByte(' ')
				}
			}
			if n.Type == html.ElementNode && n.Data == "a" && len(queue) < 100 {
				for _, a := range n.Attr {
					if a.Key == "href" {
						u, e := resp.Request.URL.Parse(a.Val)
						if e == nil && u.Scheme == base.Scheme && u.Host == base.Host && u.RawQuery == "" {
							u.Fragment = ""
							queue = append(queue, u.String())
						}
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
		pages = append(pages, Page{next, title, strings.Join(strings.Fields(text.String()), " ")})
	}
	return pages, nil
}
