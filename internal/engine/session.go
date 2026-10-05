package engine

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxCookieKeys = 512
const maxCookieBytes = 64 << 10

type cookieKey struct{ name, domain, path string }
type boundedJar struct {
	mu       sync.Mutex
	jar      *cookiejar.Jar
	keys     map[cookieKey]int
	bytes    int
	rejected int64
}

func (j *boundedJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.jar.Cookies(u)
}

func (j *boundedJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	accepted := make([]*http.Cookie, 0, min(len(cookies), maxCookieKeys))
	for _, c := range cookies {
		domain := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
		if domain == "" {
			domain = strings.ToLower(u.Hostname())
		}
		path := c.Path
		if !strings.HasPrefix(path, "/") {
			path = "/"
			if at := strings.LastIndex(u.Path, "/"); at > 0 {
				path = u.Path[:at]
			}
		}
		key := cookieKey{c.Name, domain, path}
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.MaxAge <= 0 && c.Expires.Before(time.Now())) {
			j.bytes -= j.keys[key]
			delete(j.keys, key)
			accepted = append(accepted, c)
			continue
		}
		old, exists := j.keys[key]
		size := len(c.Name) + len(c.Value) + len(domain) + len(path)
		if (!exists && len(j.keys) >= maxCookieKeys) || j.bytes-old+size > maxCookieBytes {
			j.rejected++
			continue
		}
		copyCookie := *c
		copyCookie.Name = strings.Clone(c.Name)
		copyCookie.Value = strings.Clone(c.Value)
		copyCookie.Domain = strings.Clone(c.Domain)
		copyCookie.Path = strings.Clone(c.Path)
		copyCookie.Raw = ""
		copyCookie.RawExpires = ""
		copyCookie.Unparsed = nil
		key = cookieKey{copyCookie.Name, strings.Clone(domain), strings.Clone(path)}
		j.keys[key] = size
		j.bytes += size - old
		accepted = append(accepted, &copyCookie)
	}
	j.jar.SetCookies(u, accepted)
}

func (j *boundedJar) Rejections() int64 { j.mu.Lock(); defer j.mu.Unlock(); return j.rejected }
