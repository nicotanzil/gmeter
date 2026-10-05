package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const MaxWorkers = 32

// Worker control URLs must not expose bearer credentials over a remote HTTP
// connection. The target HTTP URL remains independent of the control channel.
func WorkerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("worker URL must be an HTTP(S) origin without credentials, query, or path")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("remote worker control requires HTTPS; HTTP is permitted only on a literal loopback IP")
		}
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("worker URL port must be between 1 and 65535")
		}
	}
	return nil
}

func (c Config) validateWorkers() error {
	if len(c.Workers) == 0 {
		return nil
	}
	if len(c.Workers) > MaxWorkers || c.Mode != "rate" || len(c.Workers) > c.MaxInFlight {
		return fmt.Errorf("distributed runs require rate mode and 1–32 workers, with at least one concurrency slot per worker")
	}
	seen := map[string]bool{}
	for i, raw := range c.Workers {
		if err := WorkerURL(raw); err != nil {
			return fmt.Errorf("workers[%d]: %w", i, err)
		}
		u, _ := url.Parse(raw)
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		key := u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
		if seen[key] {
			return fmt.Errorf("worker origins must be unique")
		}
		seen[key] = true
	}
	return nil
}
