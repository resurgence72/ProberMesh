package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	cfg "probermesh/config"
	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	pconfig "github.com/prometheus/common/config"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"
)

type byteCounter struct {
	io.ReadCloser
	n int64
}

func probeHTTP(ctx context.Context, target string, httpProbe cfg.HTTPProbe, sourceRegion, targetRegion string) *pb.PorberResultReq {
	var (
		redirects int
		module    = httpProbe

		defaultHTTPProberResultReq = &pb.PorberResultReq{
			ProberType:   util.ProbeHTTPType,
			ProberTarget: target,
			LocalIP:      agentIP,
			SourceRegion: sourceRegion,
			TargetRegion: targetRegion,
			HTTPFields:   make(map[string]float64),
			TLSFields:    make(map[string]string),
		}
	)

	httpConfig := module

	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	targetURL, err := url.Parse(target)
	if err != nil {
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}

	targetHost := targetURL.Hostname()
	targetPort := targetURL.Port()

	var ip *net.IPAddr

	var lookupTime float64
	ip, lookupTime, err = chooseProtocol(ctx, module.IPProtocol, module.IPProtocolFallback, targetHost)

	defaultHTTPProberResultReq.HTTPFields["resolve"] = lookupTime
	if err != nil {
		logrus.Errorln("resolve err ", err)
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}

	httpClientConfig := module.HTTPClientConfig
	httpClientConfig.TLSConfig.ServerName = targetHost

	// However, if there is a Host header it is better to use
	// its value instead. This helps avoid TLS handshake error
	// if targetHost is an IP address.
	for name, value := range httpConfig.Headers {
		if textproto.CanonicalMIMEHeaderKey(name) == "Host" {
			httpClientConfig.TLSConfig.ServerName = value
		}
	}

	client, err := pconfig.NewClientFromConfig(httpClientConfig, "prober_mesh", pconfig.WithKeepAlivesDisabled())
	if err != nil {
		logrus.Errorln("msg", "Error generating HTTP client", "err", err)
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}

	httpClientConfig.TLSConfig.ServerName = ""
	noServerName, err := pconfig.NewRoundTripperFromConfig(httpClientConfig, "http_probe", pconfig.WithKeepAlivesDisabled())
	if err != nil {
		logrus.Errorln("msg", "Error generating HTTP client without ServerName", "err", err)
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		logrus.Errorln("msg", "Error generating cookiejar", "err", err)
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}
	client.Jar = jar

	// Inject transport that tracks traces for each redirect,
	// and does not set TLS ServerNames on redirect if needed.
	tt := newTransport(client.Transport, noServerName)
	client.Transport = tt

	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		logrus.Debugln("msg", "Received redirect", "location", r.Response.Header.Get("Location"))
		redirects = len(via)
		if redirects > 10 || !httpConfig.HTTPClientConfig.FollowRedirects {
			logrus.Debugln("msg", "Not following redirect")
			return errors.New("don't follow redirects")
		}
		return nil
	}

	if httpConfig.Method == "" {
		httpConfig.Method = "GET"
	}

	origHost := targetURL.Host
	if ip != nil {
		// Replace the host field in the URL with the IP we resolved.
		if targetPort == "" {
			if strings.Contains(ip.String(), ":") {
				targetURL.Host = "[" + ip.String() + "]"
			} else {
				targetURL.Host = ip.String()
			}
		} else {
			targetURL.Host = net.JoinHostPort(ip.String(), targetPort)
		}
	}

	var body io.Reader
	var success bool
	// var respBodyBytes int64

	// If a body is configured, add it to the request.
	if httpConfig.Body != "" {
		body = strings.NewReader(httpConfig.Body)
	}

	request, err := http.NewRequest(httpConfig.Method, targetURL.String(), body)
	if err != nil {
		logrus.Errorln("msg", "Error creating request", "err", err)
		defaultHTTPProberResultReq.ProberFailedReason = err.Error()
		return defaultHTTPProberResultReq
	}
	request.Host = origHost
	request = request.WithContext(ctx)

	for key, value := range httpConfig.Headers {
		if textproto.CanonicalMIMEHeaderKey(key) == "Host" {
			request.Host = value
			continue
		}

		request.Header.Set(key, value)
	}

	_, hasUserAgent := request.Header["User-Agent"]
	if !hasUserAgent {
		request.Header.Set("User-Agent", "test")
	}

	trace := &httptrace.ClientTrace{
		DNSStart:             tt.DNSStart,
		DNSDone:              tt.DNSDone,
		ConnectStart:         tt.ConnectStart,
		ConnectDone:          tt.ConnectDone,
		GotConn:              tt.GotConn,
		GotFirstResponseByte: tt.GotFirstResponseByte,
		TLSHandshakeStart:    tt.TLSHandshakeStart,
		TLSHandshakeDone:     tt.TLSHandshakeDone,
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))

	resp, err := client.Do(request)
	// This is different from the usual err != nil you'd expect here because err won't be nil if redirects were
	// turned off. See https://github.com/golang/go/issues/3795
	//
	// If err == nil there should never be a case where resp is also nil, but better be safe than sorry, so check if
	// resp == nil first, and then check if there was an error.
	if resp == nil {
		resp = &http.Response{}
		if err != nil {
			logrus.Errorln("msg", "Error for HTTP request", "err", err)
			defaultHTTPProberResultReq.ProberFailedReason = err.Error()
			return defaultHTTPProberResultReq
		}
	} else {
		requestErrored := (err != nil)

		logrus.Debugln("msg", "Received HTTP response", "status_code", resp.StatusCode)
		if len(httpConfig.ValidStatusCodes) != 0 {
			for _, code := range httpConfig.ValidStatusCodes {
				if resp.StatusCode == code {
					success = true
					break
				}
			}
			if !success {
				logrus.Debugln("msg", "Invalid HTTP response status code", "status_code", resp.StatusCode,
					"valid_status_codes", fmt.Sprintf("%v", httpConfig.ValidStatusCodes))
			}
		} else if 200 <= resp.StatusCode && resp.StatusCode < 300 {
			success = true
		} else {
			logrus.Debugln("msg", "Invalid HTTP response status code, wanted 2xx", "status_code", resp.StatusCode)
		}

		if !success {
			defaultHTTPProberResultReq.ProberFailedReason = fmt.Sprintf("invalid resp code: %d", resp.StatusCode)
			return defaultHTTPProberResultReq
		}

		if success && (len(httpConfig.FailIfHeaderMatchesRegexp) > 0 || len(httpConfig.FailIfHeaderNotMatchesRegexp) > 0) {
			if !matchRegularExpressionsOnHeaders(resp.Header, httpConfig) {
				defaultHTTPProberResultReq.ProberFailedReason = "resp header match reg failed"
				return defaultHTTPProberResultReq
			}
		}

		// Since the configuration specifies a compression algorithm, blindly treat the response body as a
		// compressed payload; if we cannot decompress it it's a failure because the configuration says we
		// should expect the response to be compressed in that way.
		if httpConfig.Compression != "" {
			dec, err := getDecompressionReader(httpConfig.Compression, resp.Body)
			if err != nil {
				logrus.Debugln("msg", "Failed to get decompressor for HTTP response body", "err", err)
				success = false
				defaultHTTPProberResultReq.ProberFailedReason = err.Error()
				return defaultHTTPProberResultReq
			} else if dec != nil {
				// Since we are replacing the original resp.Body with the decoder, we need to make sure
				// we close the original body. We cannot close it right away because the decompressor
				// might not have read it yet.
				defer func(c io.Closer) {
					err := c.Close()
					if err != nil {
						// At this point we cannot really do anything with this error, but log
						// it in case it contains useful information as to what's the problem.
						logrus.Debugln("msg", "Error while closing response from server", "err", err)
					}
				}(resp.Body)

				resp.Body = dec
			}
		}

		// // If there's a configured body_size_limit, wrap the body in the response in a http.MaxBytesReader.
		// // This will read up to BodySizeLimit bytes from the body, and return an error if the response is
		// // larger. It forwards the Close call to the original resp.Body to make sure the TCP connection is
		// // correctly shut down. The limit is applied _after decompression_ if applicable.
		if httpConfig.BodySizeLimit > 0 {
			resp.Body = http.MaxBytesReader(nil, resp.Body, int64(httpConfig.BodySizeLimit))
		}

		byteCounter := &byteCounter{ReadCloser: resp.Body}

		if success && (len(httpConfig.FailIfBodyMatchesRegexp) > 0 || len(httpConfig.FailIfBodyNotMatchesRegexp) > 0) {
			success = matchRegularExpressions(byteCounter, httpConfig)
			logrus.Debugln("probeFailedDueToRegex ", success)
		}

		if !requestErrored {
			_, err = io.Copy(io.Discard, byteCounter)
			if err != nil {
				logrus.Debugln("msg", "Failed to read HTTP response body", "err", err)
				success = false
			}

			if err := byteCounter.Close(); err != nil {
				// We have already read everything we could from the server, maybe even uncompressed the
				// body. The error here might be either a decompression error or a TCP error. Log it in
				// case it contains useful information as to what's the problem.
				logrus.Debugln("msg", "Error while closing response from server", "error", err.Error())
			}
		}

		// At this point body is fully read and we can write end time.
		tt.current.end = time.Now()

		if err != nil {
			logrus.Errorln("msg", "Error parsing version number from HTTP version", "err", err)
		}

		if len(httpConfig.ValidHTTPVersions) != 0 {
			found := false
			for _, version := range httpConfig.ValidHTTPVersions {
				if version == resp.Proto {
					found = true
					break
				}
			}
			if !found {
				logrus.Errorln("msg", "Invalid HTTP version number", "version", resp.Proto)
				success = false
				defaultHTTPProberResultReq.ProberFailedReason = fmt.Sprintf("invalid http version number: %s", resp.Proto)
				return defaultHTTPProberResultReq
			}
		}
	}

	tt.mu.Lock()
	defer tt.mu.Unlock()
	for i, trace := range tt.traces {
		logrus.Debugln(
			"msg", "Response timings for roundtrip",
			"roundtrip", i,
			"start", trace.start,
			"dnsDone", trace.dnsDone,
			"connectDone", trace.connectDone,
			"gotConn", trace.gotConn,
			"responseStart", trace.responseStart,
			"tlsStart", trace.tlsStart,
			"tlsDone", trace.tlsDone,
			"end", trace.end,
		)
		// We get the duration for the first request from chooseProtocol.
		if i != 0 {
			defaultHTTPProberResultReq.HTTPFields["resolve"] = trace.dnsDone.Sub(trace.start).Seconds()
		}
		// Continue here if we never got a connection because a request failed.
		if trace.gotConn.IsZero() {
			continue
		}
		if trace.tls {
			// dnsDone must be set if gotConn was set.
			defaultHTTPProberResultReq.HTTPFields["connect"] = trace.connectDone.Sub(trace.dnsDone).Seconds()
			defaultHTTPProberResultReq.HTTPFields["tls"] = trace.tlsDone.Sub(trace.tlsStart).Seconds()

			// tls info
			defaultHTTPProberResultReq.TLSFields["version"] = getTLSVersion(resp.TLS)
			defaultHTTPProberResultReq.TLSFields["expiry"] = strconv.FormatInt(getEarliestCertExpiry(resp.TLS).Unix(), 10)

		} else {
			defaultHTTPProberResultReq.HTTPFields["connect"] = trace.gotConn.Sub(trace.dnsDone).Seconds()
		}

		// Continue here if we never got a response from the server.
		if trace.responseStart.IsZero() {
			continue
		}
		defaultHTTPProberResultReq.HTTPFields["processing"] = trace.responseStart.Sub(trace.gotConn).Seconds()

		// Continue here if we never read the full response from the server.
		// Usually this means that request either failed or was redirected.
		if trace.end.IsZero() {
			continue
		}
		defaultHTTPProberResultReq.HTTPFields["transfer"] = trace.end.Sub(trace.responseStart).Seconds()
	}

	if resp.TLS != nil {
		if httpConfig.FailIfSSL {
			logrus.Errorln("msg", "Final request was over SSL")
			success = false
			defaultHTTPProberResultReq.ProberFailedReason = "Final request was over SSL"
			return defaultHTTPProberResultReq
		}
	} else if httpConfig.FailIfNotSSL {
		logrus.Errorln("msg", "Final request was not over SSL")
		success = false
		defaultHTTPProberResultReq.ProberFailedReason = "Final request was not over SSL"
		return defaultHTTPProberResultReq
	}

	if success {
		defaultHTTPProberResultReq.ProberSuccess = true
	}
	return defaultHTTPProberResultReq
}
