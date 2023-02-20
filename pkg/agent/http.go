package agent

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/andybalholm/brotli"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

func chooseProtocol(ctx context.Context, IPProtocol string, fallbackIPProtocol bool, target string) (ip *net.IPAddr, lookupTime float64, err error) {
	logrus.Infoln("msg", "Resolving target address", "target", target, "ip_protocol", IPProtocol)
	resolveStart := time.Now()

	defer func() {
		lookupTime = time.Since(resolveStart).Seconds()
		//probeDNSLookupTimeSeconds.Add(lookupTime)
	}()

	resolver := &net.Resolver{}
	if !fallbackIPProtocol {
		ips, err := resolver.LookupIP(ctx, IPProtocol, target)
		if err == nil {
			for _, ip := range ips {
				//level.Info(logger).Log("msg", "Resolved target address", "target", target, "ip", ip.String())
				//probeIPProtocolGauge.Set(protocolToGauge[IPProtocol])
				//probeIPAddrHash.Set(ipHash(ip))
				return &net.IPAddr{IP: ip}, lookupTime, nil
			}
		}
		//level.Error(logger).Log("msg", "Resolution with IP protocol failed", "target", target, "ip_protocol", IPProtocol, "err", err)
		fmt.Println("Resolution with IP protocol failed", "target", target, "ip_protocol", IPProtocol, "err", err)
		return nil, 0.0, err
	}

	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil {
		//level.Error(logger).Log("msg", "Resolution with IP protocol failed", "target", target, "err", err)
		fmt.Println("Resolution with IP protocol failed", "target", target, "err", err)
		return nil, 0.0, err
	}

	// Return the IP in the requested protocol.
	var fallback *net.IPAddr
	for _, ip := range ips {
		switch IPProtocol {
		case "ip4":
			if ip.IP.To4() != nil {
				//level.Info(logger).Log("msg", "Resolved target address", "target", target, "ip", ip.String())
				//probeIPProtocolGauge.Set(4)
				//probeIPAddrHash.Set(ipHash(ip.IP))
				return &ip, lookupTime, nil
			}

			// ip4 as fallback
			fallback = &ip

		case "ip6":
			if ip.IP.To4() == nil {
				//level.Info(logger).Log("msg", "Resolved target address", "target", target, "ip", ip.String())
				//probeIPProtocolGauge.Set(6)
				//probeIPAddrHash.Set(ipHash(ip.IP))
				return &ip, lookupTime, nil
			}

			// ip6 as fallback
			fallback = &ip
		}
	}

	// Unable to find ip and no fallback set.
	if fallback == nil || !fallbackIPProtocol {
		return nil, 0.0, fmt.Errorf("unable to find ip; no fallback")
	}

	logrus.Infoln("msg", "Resolved target address", "target", target, "ip", fallback.String())
	return fallback, lookupTime, nil
}

func newTransport(rt, noServerName http.RoundTripper, ) *transport {
	return &transport{
		Transport:             rt,
		NoServerNameTransport: noServerName,
		traces:                []*roundTripTrace{},
	}
}

type transport struct {
	Transport             http.RoundTripper
	NoServerNameTransport http.RoundTripper
	firstHost             string

	mu      sync.Mutex
	traces  []*roundTripTrace
	current *roundTripTrace
}
type roundTripTrace struct {
	tls           bool
	start         time.Time
	dnsDone       time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	end           time.Time
	tlsStart      time.Time
	tlsDone       time.Time
}

func (t *transport) DNSStart(_ httptrace.DNSStartInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.start = time.Now()
}
func (t *transport) DNSDone(_ httptrace.DNSDoneInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.dnsDone = time.Now()
}
func (ts *transport) ConnectStart(_, _ string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t := ts.current
	// No DNS resolution because we connected to IP directly.
	if t.dnsDone.IsZero() {
		t.start = time.Now()
		t.dnsDone = t.start
	}
}
func (t *transport) ConnectDone(net, addr string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.connectDone = time.Now()
}
func (t *transport) GotConn(_ httptrace.GotConnInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.gotConn = time.Now()
}
func (t *transport) GotFirstResponseByte() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.responseStart = time.Now()
}
func (t *transport) TLSHandshakeStart() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.tlsStart = time.Now()
}
func (t *transport) TLSHandshakeDone(_ tls.ConnectionState, _ error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.tlsDone = time.Now()
}
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	//level.Info(t.logger).Log("msg", "Making HTTP request", "url", req.URL.String(), "host", req.Host)

	trace := &roundTripTrace{}
	if req.URL.Scheme == "https" {
		trace.tls = true
	}
	t.current = trace
	t.traces = append(t.traces, trace)

	if t.firstHost == "" {
		t.firstHost = req.URL.Host
	}

	if t.firstHost != req.URL.Host {
		// This is a redirect to something other than the initial host,
		// so TLS ServerName should not be set.
		//level.Info(t.logger).Log("msg", "Address does not match first address, not sending TLS ServerName", "first", t.firstHost, "address", req.URL.Host)
		return t.NoServerNameTransport.RoundTrip(req)
	}

	return t.Transport.RoundTrip(req)
}

func matchRegularExpressionsOnHeaders(header http.Header, httpConfig HTTPProbe) bool {
	for _, headerMatchSpec := range httpConfig.FailIfHeaderMatchesRegexp {
		values := header[textproto.CanonicalMIMEHeaderKey(headerMatchSpec.Header)]
		if len(values) == 0 {
			if !headerMatchSpec.AllowMissing {
				//level.Error(logger).Log("msg", "Missing required header", "header", headerMatchSpec.Header)
				return false
			} else {
				continue // No need to match any regex on missing headers.
			}
		}

		for _, val := range values {
			if headerMatchSpec.Regexp.MatchString(val) {
				//level.Error(logger).Log("msg", "Header matched regular expression", "header", headerMatchSpec.Header,
				//	"regexp", headerMatchSpec.Regexp, "value_count", len(values))
				return false
			}
		}
	}
	for _, headerMatchSpec := range httpConfig.FailIfHeaderNotMatchesRegexp {
		values := header[textproto.CanonicalMIMEHeaderKey(headerMatchSpec.Header)]
		if len(values) == 0 {
			if !headerMatchSpec.AllowMissing {
				//level.Error(logger).Log("msg", "Missing required header", "header", headerMatchSpec.Header)
				return false
			} else {
				continue // No need to match any regex on missing headers.
			}
		}

		anyHeaderValueMatched := false

		for _, val := range values {
			if headerMatchSpec.Regexp.MatchString(val) {
				anyHeaderValueMatched = true
				break
			}
		}

		if !anyHeaderValueMatched {
			//level.Error(logger).Log("msg", "Header did not match regular expression", "header", headerMatchSpec.Header,
			//	"regexp", headerMatchSpec.Regexp, "value_count", len(values))
			return false
		}
	}

	return true
}

func getDecompressionReader(algorithm string, origBody io.ReadCloser) (io.ReadCloser, error) {
	switch strings.ToLower(algorithm) {
	case "br":
		return io.NopCloser(brotli.NewReader(origBody)), nil

	case "deflate":
		return flate.NewReader(origBody), nil

	case "gzip":
		return gzip.NewReader(origBody)

	case "identity", "":
		return origBody, nil

	default:
		return nil, errors.New("unsupported compression algorithm")
	}
}

func matchRegularExpressions(reader io.Reader, httpConfig HTTPProbe) bool {
	body, err := io.ReadAll(reader)
	if err != nil {
		//level.Error(logger).Log("msg", "Error reading HTTP body", "err", err)
		return false
	}
	for _, expression := range httpConfig.FailIfBodyMatchesRegexp {
		if expression.Regexp.Match(body) {
			//level.Error(logger).Log("msg", "Body matched regular expression", "regexp", expression)
			return false
		}
	}
	for _, expression := range httpConfig.FailIfBodyNotMatchesRegexp {
		if !expression.Regexp.Match(body) {
			//level.Error(logger).Log("msg", "Body did not match regular expression", "regexp", expression)
			return false
		}
	}
	return true
}

func getTLSVersion(state *tls.ConnectionState) string {
	switch state.Version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "unknown"
	}
}

func getEarliestCertExpiry(state *tls.ConnectionState) time.Time {
	earliest := time.Time{}
	for _, cert := range state.PeerCertificates {
		if (earliest.IsZero() || cert.NotAfter.Before(earliest)) && !cert.NotAfter.IsZero() {
			earliest = cert.NotAfter
		}
	}
	return earliest
}
