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

	"github.com/alecthomas/units"
	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v3"
)

type ProberMeshConfig struct {
	ProberConfigs []*ProberConfig `yaml:"prober_configs"`
}

type ProberConfig struct {
	ProberType string    `yaml:"prober_type"`
	Region     string    `yaml:"region"`
	HttpProbe  HTTPProbe `yaml:"http"`
	Targets    []string  `yaml:"targets"`
}

func (p *ProberConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	pc := &ProberConfig{}
	type plain ProberConfig

	if err := unmarshal((*plain)(pc)); err != nil {
		return err
	}

	if len(pc.HttpProbe.IPProtocol) == 0 {
		pc.HttpProbe.IPProtocol = DefaultHttpProbe.IPProtocol
	}

	*p = *pc
	return nil
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
	HTTPClientConfig             config.HTTPClientConfig `yaml:"http_client_config,inline"`
	Compression                  string                  `yaml:"compression,omitempty"`
	BodySizeLimit                units.Base2Bytes        `yaml:"body_size_limit,omitempty"`
}

var (
	DefaultHttpProbe = HTTPProbe{
		IPProtocolFallback: true,
		HTTPClientConfig:   config.DefaultHTTPClientConfig,
		IPProtocol:         "ip4",
		ValidStatusCodes:   []int{200},
	}
	filePath string
	m        sync.Mutex
)

func (s *HTTPProbe) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*s = DefaultHttpProbe
	type plain HTTPProbe
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}

	// BodySizeLimit == 0 means no limit. By leaving it at 0 we
	// avoid setting up the limiter.
	if s.BodySizeLimit < 0 || s.BodySizeLimit == math.MaxInt64 {
		// The implementation behind http.MaxBytesReader tries
		// to add 1 to the specified limit causing it to wrap
		// around and become negative, and then it tries to use
		// that result to index an slice.
		s.BodySizeLimit = math.MaxInt64 - 1
	}

	if err := s.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	if s.NoFollowRedirects != nil {
		s.HTTPClientConfig.FollowRedirects = !*s.NoFollowRedirects
	}

	for key, value := range s.Headers {
		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Accept-Encoding":
			if !isCompressionAcceptEncodingValid(s.Compression, value) {
				return fmt.Errorf(`invalid configuration "%s: %s", "compression: %s"`, key, value, s.Compression)
			}
		}
	}

	if len(s.IPProtocol) == 0 {
		s.IPProtocol = DefaultHttpProbe.IPProtocol
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
	original string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return fmt.Errorf("\"Could not compile regular expression\" regexp=\"%s\"", s)
	}
	*re = r
	return nil
}

func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile(s)
	return Regexp{
		Regexp:   regex,
		original: s,
	}, err
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.original != "" {
		return re.original, nil
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
	cfg := &ProberMeshConfig{}
	err := yaml.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
