package metrics

import "time"

// HTTP timings describe observed transport events. Their sample populations
// differ: reused connections have no DNS, TCP, or TLS sample. Timings overlap
// and must not be added together to reconstruct request latency.
type HTTPMetrics struct {
	ConnectionRequests  int64            `json:"connection_requests"`
	ConnectionsAcquired int64            `json:"connections_acquired"`
	ReusedConnections   int64            `json:"reused_connections"`
	IdleReuses          int64            `json:"idle_reuses"`
	DNSLookups          int64            `json:"dns_lookups"`
	DNSFailures         int64            `json:"dns_failures"`
	DialAttempts        int64            `json:"dial_attempts"`
	DialFailures        int64            `json:"dial_failures"`
	TLSHandshakes       int64            `json:"tls_handshakes"`
	TLSFailures         int64            `json:"tls_failures"`
	WriteCompletions    int64            `json:"write_completions"`
	WriteFailures       int64            `json:"write_failures"`
	FirstHeaderBytes    int64            `json:"first_header_bytes"`
	ResponseHeaders     int64            `json:"response_headers"`
	BodyReadFailures    int64            `json:"body_read_failures"`
	UnfinishedEvents    int64            `json:"unfinished_events"`
	Protocols           map[string]int64 `json:"protocols"`
	ConnectionWait      Latency          `json:"connection_wait"`
	DNS                 Latency          `json:"dns"`
	Connect             Latency          `json:"connect"`
	TLS                 Latency          `json:"tls"`
	Write               Latency          `json:"write"`
	FirstByte           Latency          `json:"first_byte"`
	BodyRead            Latency          `json:"body_read"`
}

type HTTPEventKind uint8

const (
	HTTPGetConn HTTPEventKind = iota
	HTTPGotConn
	HTTPDNSStart
	HTTPDNSDone
	HTTPConnectStart
	HTTPConnectDone
	HTTPTLSStart
	HTTPTLSDone
	HTTPWroteRequest
	HTTPFirstByte
	HTTPHeaders
	HTTPBodyRead
	HTTPUnfinished
)

type HTTPEvent struct {
	Kind     HTTPEventKind
	Duration time.Duration
	Timed    bool
	Failed   bool
	Reused   bool
	Idle     bool
	Protocol string
}

type httpHistograms struct {
	counts                                          HTTPMetrics
	wait, dns, connect, tls, write, firstByte, body Histogram
}

// ObserveHTTP uses the collector's existing lock. Histograms are allocated only
// on an actual transport event, not for journey-only or unvisited collectors.
func (c *Collector) ObserveHTTP(e HTTPEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.http == nil {
		c.http = &httpHistograms{counts: HTTPMetrics{Protocols: map[string]int64{}}}
	}
	h := c.http
	n := &h.counts
	record := func(hist *Histogram) {
		if e.Timed {
			hist.Record(e.Duration)
		}
	}
	failed := func(value *int64) {
		if e.Failed {
			(*value)++
		}
	}
	switch e.Kind {
	case HTTPGetConn:
		n.ConnectionRequests++
	case HTTPGotConn:
		n.ConnectionsAcquired++
		if e.Reused {
			n.ReusedConnections++
		}
		if e.Reused && e.Idle {
			n.IdleReuses++
		}
		record(&h.wait)
	case HTTPDNSStart:
		n.DNSLookups++
	case HTTPDNSDone:
		failed(&n.DNSFailures)
		record(&h.dns)
	case HTTPConnectStart:
		n.DialAttempts++
	case HTTPConnectDone:
		failed(&n.DialFailures)
		record(&h.connect)
	case HTTPTLSStart:
		n.TLSHandshakes++
	case HTTPTLSDone:
		failed(&n.TLSFailures)
		record(&h.tls)
	case HTTPWroteRequest:
		n.WriteCompletions++
		failed(&n.WriteFailures)
		record(&h.write)
	case HTTPFirstByte:
		n.FirstHeaderBytes++
		record(&h.firstByte)
	case HTTPHeaders:
		n.ResponseHeaders++
		protocol := e.Protocol
		if protocol != "HTTP/1.0" && protocol != "HTTP/1.1" && protocol != "HTTP/2.0" {
			protocol = "other"
		}
		n.Protocols[protocol]++
	case HTTPBodyRead:
		failed(&n.BodyReadFailures)
		record(&h.body)
	case HTTPUnfinished:
		n.UnfinishedEvents++
	}
}

func (h *httpHistograms) snapshot() *HTTPMetrics {
	if h == nil {
		return nil
	}
	n := h.counts
	n.Protocols = make(map[string]int64, len(h.counts.Protocols))
	for protocol, count := range h.counts.Protocols {
		n.Protocols[protocol] = count
	}
	n.ConnectionWait = h.wait.snapshot()
	n.DNS = h.dns.snapshot()
	n.Connect = h.connect.snapshot()
	n.TLS = h.tls.snapshot()
	n.Write = h.write.snapshot()
	n.FirstByte = h.firstByte.snapshot()
	n.BodyRead = h.body.snapshot()
	return &n
}
