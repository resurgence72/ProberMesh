package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/units"
	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v3"

	"probermesh/pkg/util"
)

type ProberMeshConfig struct {
	ProberConfigs []*ProberConfig `yaml:"prober_configs"`
}

type ICMPProbe struct {
	PayloadSize int    `yaml:"payload_size"`
	PacketCount int    `yaml:"packet_count"`
	Interval    string `yaml:"interval"`
	IntervalDur time.Duration
}

type ProberConfig struct {
	ProberType string     `yaml:"prober_type"`
	Region     string     `yaml:"region"`
	HttpProbe  *HTTPProbe `yaml:"http"`
	ICMPProbe  *ICMPProbe `yaml:"icmp"`
	Targets    []string   `yaml:"targets"`
}

func (p *ProberConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	pc := &ProberConfig{}
	type plain ProberConfig
	if err := unmarshal((*plain)(pc)); err != nil {
		return err
	}

	if pc.ProberType == "icmp" && pc.ICMPProbe == nil {
		pc.ICMPProbe = defaultICMPProbe
	}

	if pc.ProberType == "http" && pc.HttpProbe == nil {
		pc.HttpProbe = defaultHttpProbe
		compileProbeRegexp(pc.HttpProbe)
	}

	*p = *pc
	return nil
}

func compileProbeRegexp(probe *HTTPProbe) {
	for i := range probe.FailIfBodyMatchesRegexp {
		regexp, _ := newRegexp(probe.FailIfBodyMatchesRegexp[i].Original)
		probe.FailIfBodyMatchesRegexp[i] = regexp
	}

	for i := range probe.FailIfBodyNotMatchesRegexp {
		regexp, _ := newRegexp(probe.FailIfBodyNotMatchesRegexp[i].Original)
		probe.FailIfBodyNotMatchesRegexp[i] = regexp
	}

	for i := range probe.FailIfHeaderMatchesRegexp {
		regexp, _ := newRegexp(probe.FailIfHeaderMatchesRegexp[i].Regexp.Original)
		probe.FailIfHeaderMatchesRegexp[i].Regexp = regexp
	}

	for i := range probe.FailIfHeaderNotMatchesRegexp {
		regexp, _ := newRegexp(probe.FailIfHeaderNotMatchesRegexp[i].Regexp.Original)
		probe.FailIfHeaderNotMatchesRegexp[i].Regexp = regexp
	}
}

type HTTPProbe struct {
	// Defaults to 2xx.
	ValidStatusCodes             []int                   `yaml:"valid_status_codes,omitempty"`
	ValidHTTPVersions            []string                `yaml:"valid_http_versions,omitempty"`
	IPProtocol                   string                  `yaml:"preferred_ip_protocol,omitempty"`
	IPProtocolFallback           bool                    `yaml:"ip_protocol_fallback,omitempty"`
	SkipResolvePhaseWithProxy    bool                    `yaml:"skip_resolve_phase_with_proxy,omitempty"`
	NoFollowRedirects            *bool                   `yaml:"no_follow_redirects,omitempty"`
	FailIfSSL                    bool                    `yaml:"fail_if_ssl,omitempty"`
	FailIfNotSSL                 bool                    `yaml:"fail_if_not_ssl,omitempty"`
	Method                       string                  `yaml:"method,omitempty"`
	Headers                      map[string]string       `yaml:"headers,omitempty"`
	FailIfBodyMatchesRegexp      []Regexp                `yaml:"fail_if_body_matches_regexp,omitempty"`
	FailIfBodyNotMatchesRegexp   []Regexp                `yaml:"fail_if_body_not_matches_regexp,omitempty"`
	FailIfHeaderMatchesRegexp    []HeaderMatch           `yaml:"fail_if_header_matches,omitempty"`
	FailIfHeaderNotMatchesRegexp []HeaderMatch           `yaml:"fail_if_header_not_matches,omitempty"`
	Body                         string                  `yaml:"body,omitempty"`
	HTTPClientConfig             config.HTTPClientConfig `yaml:"http_client_config,omitempty"`
	Compression                  string                  `yaml:"compression,omitempty"`
	BodySizeLimit                units.Base2Bytes        `yaml:"body_size_limit,omitempty"`
}

var (
	defaultHttpProbe = &HTTPProbe{
		IPProtocolFallback: true,
		HTTPClientConfig:   config.DefaultHTTPClientConfig,
		IPProtocol:         "ip4",
		ValidStatusCodes:   []int{200},
	}

	defaultICMPProbe = &ICMPProbe{
		PacketCount: 64,
		PayloadSize: 64,
		Interval:    "15ms",
		IntervalDur: time.Duration(15) * time.Millisecond,
	}
	filePath string
	m        sync.Mutex
)

func (i *ICMPProbe) UnmarshalYAML(unmarshal func(interface{}) error) error {
	ip := &ICMPProbe{}
	type plain ICMPProbe
	if err := unmarshal((*plain)(ip)); err != nil {
		return err
	}

	if ip.PacketCount <= 0 {
		ip.PacketCount = 64
	}

	if ip.PayloadSize <= 0 {
		ip.PayloadSize = 64
	}

	if len(ip.Interval) == 0 {
		ip.Interval = "15ms"
	}
	dur, err := util.ParseDuration(ip.Interval)
	if err != nil {
		return err
	}
	ip.IntervalDur = dur

	*i = *ip
	return nil
}

func (hp *HTTPProbe) UnmarshalYAML(unmarshal func(interface{}) error) error {
	h := defaultHttpProbe
	type plain HTTPProbe
	if err := unmarshal((*plain)(h)); err != nil {
		return err
	}

	// BodySizeLimit == 0 means no limit. By leaving it at 0 we
	// avoid setting up the limiter.
	if h.BodySizeLimit < 0 || h.BodySizeLimit == math.MaxInt64 {
		// The implementation behind http.MaxBytesReader tries
		// to add 1 to the specified limit causing it to wrap
		// around and become negative, and then it tries to use
		// that result to index an slice.
		h.BodySizeLimit = math.MaxInt64 - 1
	}

	if err := h.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	if h.NoFollowRedirects != nil {
		h.HTTPClientConfig.FollowRedirects = !*h.NoFollowRedirects
	}

	for key, value := range h.Headers {
		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Accept-Encoding":
			if !isCompressionAcceptEncodingValid(h.Compression, value) {
				return fmt.Errorf(`invalid configuration "%h: %h", "compression: %h"`, key, value, h.Compression)
			}
		}
	}

	if len(h.IPProtocol) == 0 {
		h.IPProtocol = defaultHttpProbe.IPProtocol
	}

	if len(h.IPProtocol) == 0 {
		h.IPProtocol = defaultHttpProbe.IPProtocol
	}
	return nil
}

func isCompressionAcceptEncodingValid(encoding, acceptEncoding string) bool {
	// unspecified compression + any encoding value is valid
	// any compression + no accept encoding is valid
	if encoding == "" || acceptEncoding == "" {
		return true
	}

	type encodingQuality struct {
		encoding string
		quality  float32
	}

	var encodings []encodingQuality

	for _, parts := range strings.Split(acceptEncoding, ",") {
		var e encodingQuality

		if idx := strings.LastIndexByte(parts, ';'); idx == -1 {
			e.encoding = strings.TrimSpace(parts)
			e.quality = 1.0
		} else {
			parseQuality := func(str string) float32 {
				q, err := strconv.ParseFloat(str, 32)
				if err != nil {
					return 0
				}
				return float32(math.Round(q*1000) / 1000)
			}

			e.encoding = strings.TrimSpace(parts[:idx])

			q := strings.TrimSpace(parts[idx+1:])
			q = strings.TrimPrefix(q, "q=")
			e.quality = parseQuality(q)
		}

		encodings = append(encodings, e)
	}

	sort.SliceStable(encodings, func(i, j int) bool {
		return encodings[j].quality < encodings[i].quality
	})

	for _, e := range encodings {
		if encoding == e.encoding || e.encoding == "*" {
			return e.quality > 0
		}
	}

	return false
}

type Regexp struct {
	*regexp.Regexp
	Original string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	_, err := regexp.Compile(s)
	if err != nil {
		return fmt.Errorf("\"Could not compile regular expression\" regexp=\"%s\"", s)
	}
	*re = Regexp{Original: s}
	return nil
}

func newRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile(s)
	return Regexp{
		Regexp:   regex,
		Original: s,
	}, err
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.Original != "" {
		return re.Original, nil
	}
	return nil, nil
}

type HeaderMatch struct {
	Header       string `yaml:"header,omitempty"`
	Regexp       Regexp `yaml:"regexp,omitempty"`
	AllowMissing bool   `yaml:"allow_missing,omitempty"`
}

func (s *HeaderMatch) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain HeaderMatch
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}

	if s.Header == "" {
		return errors.New("header name must be set for HTTP header matchers")
	}

	if s.Regexp.Regexp == nil || s.Regexp.Regexp.String() == "" {
		return errors.New("regexp must be set for HTTP header matchers")
	}

	return nil
}

var cfg *ProberMeshConfig

func InitConfig(path string) error {
	meshConfig, err := loadFile(path)
	if err != nil {
		return err
	}
	cfg = meshConfig
	filePath = path
	return nil
}

func Get() *ProberMeshConfig {
	m.Lock()
	defer m.Unlock()

	if cfg == nil {
		// 防止不指定配置参数时遍历pcs报错
		return &ProberMeshConfig{ProberConfigs: nil}
	}
	return cfg
}

func loadFile(fileName string) (*ProberMeshConfig, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	return load(bytes)
}

func ReloadConfig() error {
	return InitConfig(filePath)
}

func load(bytes []byte) (*ProberMeshConfig, error) {
	c := &ProberMeshConfig{}

	err := yaml.Unmarshal(bytes, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}
