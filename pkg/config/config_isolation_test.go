package config

import "testing"

// 验证多个probe配置互相隔离，互不污染（P1-3回归测试）
func TestMultiProbeConfigIsolation(t *testing.T) {
	yaml := `
prober_configs:
  - prober_type: http
    region: region-a
    http:
      headers:
        X-Test: value-a
      valid_status_codes: [200, 204]
    targets: ["a.example.com"]
  - prober_type: http
    region: region-b
    http:
      method: POST
    targets: ["b.example.com"]
  - prober_type: icmp
    region: region-c
    targets: ["1.1.1.1"]
`

	cfg, err := load([]byte(yaml))
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if len(cfg.ProberConfigs) != 3 {
		t.Fatalf("expect 3 prober configs, got %d", len(cfg.ProberConfigs))
	}

	a := cfg.ProberConfigs[0]
	b := cfg.ProberConfigs[1]

	if len(a.HttpProbe.Headers) != 1 || a.HttpProbe.Headers["X-Test"] != "value-a" {
		t.Fatalf("probe a headers polluted: %v", a.HttpProbe.Headers)
	}
	if len(a.HttpProbe.ValidStatusCodes) != 2 {
		t.Fatalf("probe a valid_status_codes polluted: %v", a.HttpProbe.ValidStatusCodes)
	}
	if a.HttpProbe.Method != "" {
		t.Fatalf("probe a method should default empty, got %s", a.HttpProbe.Method)
	}

	if len(b.HttpProbe.Headers) != 0 {
		t.Fatalf("probe b inherited headers from probe a: %v", b.HttpProbe.Headers)
	}
	if len(b.HttpProbe.ValidStatusCodes) != 1 || b.HttpProbe.ValidStatusCodes[0] != 200 {
		t.Fatalf("probe b inherited valid_status_codes from probe a: %v", b.HttpProbe.ValidStatusCodes)
	}
	if b.HttpProbe.Method != "POST" {
		t.Fatalf("probe b method should POST, got %s", b.HttpProbe.Method)
	}

	c := cfg.ProberConfigs[2]
	if c.ICMPProbe == nil {
		t.Fatal("icmp probe should have default ICMPProbe")
	}
	if c.ICMPProbe.PacketCount != 64 || c.ICMPProbe.IntervalDur == 0 {
		t.Fatalf("icmp probe default values wrong: %+v", c.ICMPProbe)
	}
	if a.ICMPProbe != nil {
		t.Fatal("http probe should not have ICMPProbe")
	}
}

// 验证多个icmp配置不共享同一默认值指针（P1-3回归测试）
func TestICMPProbeDefaultIsolation(t *testing.T) {
	p1 := DefaultICMPProbe()
	p2 := DefaultICMPProbe()
	if p1 == p2 {
		t.Fatal("DefaultICMPProbe should return independent copies")
	}
	p1.PacketCount = 10
	if p2.PacketCount == 10 {
		t.Fatal("DefaultICMPProbe copies share state")
	}
}
