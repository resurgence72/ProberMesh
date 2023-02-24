package agent

import (
	"context"
	"math/rand"
	"os"
	"sync"
	"time"

	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	"github.com/go-ping/ping"
	"github.com/sirupsen/logrus"
)

type ICMPProbe struct {
	IPProtocol         string `yaml:"preferred_ip_protocol,omitempty"` // Defaults to "ip6".
	IPProtocolFallback bool   `yaml:"ip_protocol_fallback,omitempty"`
	SourceIPAddress    string `yaml:"source_ip_address,omitempty"`
	PayloadSize        int    `yaml:"payload_size,omitempty"`
	DontFragment       bool   `yaml:"dont_fragment,omitempty"`
	TTL                int    `yaml:"ttl,omitempty"`
}

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// PID is typically 1 when running in a container; in that case, set
	// the ICMP echo ID to a random value to avoid potential clashes with
	// other blackbox_exporter instances. See #411.
	if pid := os.Getpid(); pid == 1 {
		icmpID = r.Intn(1 << 16)
	} else {
		icmpID = pid & 0xffff
	}

	// Start the ICMP echo sequence at a random offset to prevent them from
	// being in sync when several blackbox_exporter instances are restarted
	// at the same time. See #411.
	icmpSequence = uint16(r.Intn(1 << 16))
}

var (
	icmpID                   int
	icmpSequence             uint16
	icmpSequenceMutex        sync.Mutex
	defaultICMPTTL           = 64
	defaultICMPSize          = 64
	defaultICMPCount         = 64
	defaultICMPFloodInterval = time.Duration(15) * time.Millisecond
)

func getICMPSequence() uint16 {
	icmpSequenceMutex.Lock()
	defer icmpSequenceMutex.Unlock()
	icmpSequence++
	return icmpSequence
}

func buildDefaultICMPProbe() ICMPProbe {
	return ICMPProbe{
		IPProtocolFallback: true,
		TTL:                defaultICMPTTL,
		IPProtocol:         "ip4",
	}
}

//func probeICMP(ctx context.Context, target, sourceRegion, targetRegion string) *pb.PorberResultReq {
//	var (
//		requestType     icmp.Type
//		replyType       icmp.Type
//		icmpConn        *icmp.PacketConn
//		v4RawConn       *ipv4.RawConn
//		hopLimitFlagSet = true
//		module = buildDefaultICMPProbe()
//
//		defaultICMPPorberResultReq = &pb.PorberResultReq{
//			ProberType:    "icmp",
//			ICMPFields: make(map[string]float64),
//			SourceRegion:  sourceRegion,
//			TargetRegion:  targetRegion,
//			ProberTarget:  target,
//			LocalIP:       getLocalIP(),
//		}
//	)
//
//	dstIPAddr, lookupTime, err := chooseProtocol(ctx, module.IPProtocol, module.IPProtocolFallback, target)
//
//	if err != nil {
//		logrus.Errorln("msg", "Error resolving address", "err", err)
//		return defaultICMPPorberResultReq
//	}
//
//	// 打点resolve
//	defaultICMPPorberResultReq.ICMPFields["resolve"] = lookupTime
//
//	var srcIP net.IP
//	if len(module.SourceIPAddress) > 0 {
//		if srcIP = net.ParseIP(module.SourceIPAddress); srcIP == nil {
//			logrus.Errorln("msg", "Error parsing source ip address", "srcIP", module.SourceIPAddress)
//			return defaultICMPPorberResultReq
//		}
//		logrus.Debugln("msg", "Using source address", "srcIP", srcIP)
//	}
//
//	setupStart := time.Now()
//	logrus.Debugln("msg", "Creating socket")
//
//	privileged := true
//	// Unprivileged sockets are supported on Darwin and Linux only.
//	tryUnprivileged := runtime.GOOS == "darwin" || runtime.GOOS == "linux"
//
//	if dstIPAddr.IP.To4() == nil {
//		requestType = ipv6.ICMPTypeEchoRequest
//		replyType = ipv6.ICMPTypeEchoReply
//
//		if srcIP == nil {
//			srcIP = net.ParseIP("::")
//		}
//
//		if tryUnprivileged {
//			// "udp" here means unprivileged -- not the protocol "udp".
//			icmpConn, err = icmp.ListenPacket("udp6", srcIP.String())
//			if err != nil {
//				logrus.Debugln("msg", "Unable to do unprivileged listen on socket, will attempt privileged", "err", err)
//			} else {
//				privileged = false
//			}
//		}
//
//		if privileged {
//			icmpConn, err = icmp.ListenPacket("ip6:ipv6-icmp", srcIP.String())
//			if err != nil {
//				//logrus.Errorln("msg", "Error listening to socket", "err", err)
//				logrus.Errorln("msg", "Error listening to socket", "err", err)
//				return defaultICMPPorberResultReq
//			}
//		}
//		defer icmpConn.Close()
//
//		if err := icmpConn.IPv6PacketConn().SetControlMessage(ipv6.FlagHopLimit, true); err != nil {
//			logrus.Debugln("msg", "Failed to set Control Message for retrieving Hop Limit", "err", err)
//			hopLimitFlagSet = false
//		}
//	} else {
//		requestType = ipv4.ICMPTypeEcho
//		replyType = ipv4.ICMPTypeEchoReply
//
//		if srcIP == nil {
//			srcIP = net.ParseIP("0.0.0.0")
//		}
//
//		if module.DontFragment {
//			// If the user has set the don't fragment option we cannot use unprivileged
//			// sockets as it is not possible to set IP header level options.
//			netConn, err := net.ListenPacket("ip4:icmp", srcIP.String())
//			if err != nil {
//				logrus.Errorln("msg", "Error listening to socket", "err", err)
//				return defaultICMPPorberResultReq
//			}
//			defer netConn.Close()
//
//			v4RawConn, err = ipv4.NewRawConn(netConn)
//			if err != nil {
//				logrus.Errorln("msg", "Error creating raw connection", "err", err)
//				return defaultICMPPorberResultReq
//			}
//			defer v4RawConn.Close()
//
//			if err := v4RawConn.SetControlMessage(ipv4.FlagTTL, true); err != nil {
//				logrus.Debugln("msg", "Failed to set Control Message for retrieving TTL", "err", err)
//				hopLimitFlagSet = false
//			}
//		} else {
//			if tryUnprivileged {
//				icmpConn, err = icmp.ListenPacket("udp4", srcIP.String())
//				if err != nil {
//					logrus.Debugln("msg", "Unable to do unprivileged listen on socket, will attempt privileged", "err", err)
//				} else {
//					privileged = false
//				}
//			}
//
//			if privileged {
//				icmpConn, err = icmp.ListenPacket("ip4:icmp", srcIP.String())
//				if err != nil {
//					logrus.Errorln("msg", "Error listening to socket", "err", err)
//					return defaultICMPPorberResultReq
//				}
//			}
//			defer icmpConn.Close()
//
//			if err := icmpConn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true); err != nil {
//				logrus.Debugln("msg", "Failed to set Control Message for retrieving TTL", "err", err)
//				hopLimitFlagSet = false
//			}
//		}
//	}
//
//	var dst net.Addr = dstIPAddr
//	if !privileged {
//		dst = &net.UDPAddr{IP: dstIPAddr.IP, Zone: dstIPAddr.Zone}
//	}
//
//	var data []byte
//	if module.PayloadSize != 0 {
//		data = make([]byte, module.PayloadSize)
//		copy(data, "Prometheus Blackbox Exporter")
//	} else {
//		data = []byte("Prometheus Blackbox Exporter")
//	}
//
//	body := &icmp.Echo{
//		ID:   icmpID,
//		Seq:  int(getICMPSequence()),
//		Data: data,
//	}
//
//	logrus.Debugln("msg", "Creating ICMP packet", "seq", body.Seq, "id", body.ID)
//	wm := icmp.Message{
//		Type: requestType,
//		Code: 0,
//		Body: body,
//	}
//
//	wb, err := wm.Marshal(nil)
//	if err != nil {
//		logrus.Errorln("msg", "Error marshalling packet", "err", err)
//		return defaultICMPPorberResultReq
//	}
//
//	defaultICMPPorberResultReq.ICMPFields["setup"] = time.Since(setupStart).Seconds()
//	logrus.Debugln("msg", "Writing out packet")
//
//	rttStart := time.Now()
//
//	if icmpConn != nil {
//		ttl := module.TTL
//		if ttl > 0 {
//			if c4 := icmpConn.IPv4PacketConn(); c4 != nil {
//				logrus.Debugln("msg", "Setting TTL (IPv4 unprivileged)", "ttl", ttl)
//				c4.SetTTL(ttl)
//			}
//			if c6 := icmpConn.IPv6PacketConn(); c6 != nil {
//				logrus.Debugln("msg", "Setting TTL (IPv6 unprivileged)", "ttl", ttl)
//				c6.SetHopLimit(ttl)
//			}
//		}
//		_, err = icmpConn.WriteTo(wb, dst)
//	} else {
//		ttl := defaultICMPTTL
//		if module.TTL > 0 {
//			logrus.Debugln("msg", "Overriding TTL (raw IPv4)", "ttl", ttl)
//			ttl = module.TTL
//		}
//		// Only for IPv4 raw. Needed for setting DontFragment flag.
//		header := &ipv4.Header{
//			Version:  ipv4.Version,
//			Len:      ipv4.HeaderLen,
//			Protocol: 1,
//			TotalLen: ipv4.HeaderLen + len(wb),
//			TTL:      ttl,
//			Dst:      dstIPAddr.IP,
//			Src:      srcIP,
//		}
//
//		header.Flags |= ipv4.DontFragment
//
//		err = v4RawConn.WriteTo(header, wb, nil)
//	}
//	if err != nil {
//		logrus.Errorln("msg", "Error writing to socket", "err", err)
//		return defaultICMPPorberResultReq
//	}
//
//	// Reply should be the same except for the message type and ID if
//	// unprivileged sockets were used and the kernel used its own.
//	wm.Type = replyType
//	// Unprivileged cannot set IDs on Linux.
//	idUnknown := !privileged && runtime.GOOS == "linux"
//	if idUnknown {
//		body.ID = 0
//	}
//	wb, err = wm.Marshal(nil)
//	if err != nil {
//		logrus.Errorln("msg", "Error marshalling packet", "err", err)
//		return defaultICMPPorberResultReq
//	}
//
//	if idUnknown {
//		// If the ID is unknown (due to unprivileged sockets) we also cannot know
//		// the checksum in userspace.
//		wb[2] = 0
//		wb[3] = 0
//	}
//
//	rb := make([]byte, 65536)
//	deadline, _ := ctx.Deadline()
//	if icmpConn != nil {
//		err = icmpConn.SetReadDeadline(deadline)
//	} else {
//		err = v4RawConn.SetReadDeadline(deadline)
//	}
//	if err != nil {
//		logrus.Errorln("msg", "Error setting socket deadline", "err", err)
//		return defaultICMPPorberResultReq
//	}
//	logrus.Debugln("msg", "Waiting for reply packets")
//	for {
//		var n int
//		var peer net.Addr
//		var err error
//		var hopLimit float64 = -1
//
//		if dstIPAddr.IP.To4() == nil {
//			var cm *ipv6.ControlMessage
//			n, cm, peer, err = icmpConn.IPv6PacketConn().ReadFrom(rb)
//			// HopLimit == 0 is valid for IPv6, although go initialize it as 0.
//			if cm != nil && hopLimitFlagSet {
//				hopLimit = float64(cm.HopLimit)
//			} else {
//				logrus.Debugln("msg", "Cannot get Hop Limit from the received packet. 'probe_icmp_reply_hop_limit' will be missing.")
//			}
//		} else {
//			var cm *ipv4.ControlMessage
//			if icmpConn != nil {
//				n, cm, peer, err = icmpConn.IPv4PacketConn().ReadFrom(rb)
//			} else {
//				var h *ipv4.Header
//				var p []byte
//				h, p, cm, err = v4RawConn.ReadFrom(rb)
//				if err == nil {
//					copy(rb, p)
//					n = len(p)
//					peer = &net.IPAddr{IP: h.Src}
//				}
//			}
//			if cm != nil && hopLimitFlagSet {
//				// Not really Hop Limit, but it is in practice.
//				hopLimit = float64(cm.TTL)
//			} else {
//				logrus.Debugln("msg", "Cannot get TTL from the received packet. 'probe_icmp_reply_hop_limit' will be missing.")
//			}
//		}
//		if err != nil {
//			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
//				logrus.Errorln("msg", "Timeout reading from socket", "err", err)
//				return defaultICMPPorberResultReq
//			}
//			logrus.Errorln("msg", "Error reading from socket", "err", err)
//			continue
//		}
//		if peer.String() != dst.String() {
//			continue
//		}
//		if idUnknown {
//			// Clear the ID from the packet, as the kernel will have replaced it (and
//			// kept track of our packet for us, hence clearing is safe).
//			rb[4] = 0
//			rb[5] = 0
//		}
//		if idUnknown || replyType == ipv6.ICMPTypeEchoReply {
//			// Clear checksum to make comparison succeed.
//			rb[2] = 0
//			rb[3] = 0
//		}
//		if bytes.Equal(rb[:n], wb) {
//			defaultICMPPorberResultReq.ICMPFields["rtt"] = time.Since(rttStart).Seconds()
//			if hopLimit >= 0 {
//				logrus.Debugln("Replied packet hop limit (TTL for ipv4): ", hopLimit)
//			}
//			logrus.Debugln("msg", "Found matching reply packet")
//			defaultICMPPorberResultReq.ProberSuccess = true
//			return defaultICMPPorberResultReq
//		}
//	}
//}

func probeICMP(ctx context.Context, target, sourceRegion, targetRegion string) *pb.PorberResultReq {
	var (
		defaultICMPPorberResultReq = &pb.PorberResultReq{
			ProberType:   util.ProbeICMPType,
			ICMPFields:   make(map[string]float64),
			SourceRegion: sourceRegion,
			TargetRegion: targetRegion,
			ProberTarget: target,
			LocalIP:      agentIP,
		}
		pinger = ping.New(target)
	)
	defer pinger.Stop()

	pinger.Interval = defaultICMPFloodInterval
	pinger.RecordRtts = false
	pinger.SetNetwork("ip")
	pinger.Size = defaultICMPSize
	pinger.Count = defaultICMPCount
	pinger.Timeout = proberTimeout
	pinger.SetPrivileged(true)

	nslookup := time.Now()
	if err := pinger.Resolve(); err != nil {
		logrus.Errorln("target resolve failed ", err)
		return defaultICMPPorberResultReq
	}

	defaultICMPPorberResultReq.ICMPFields["resolve"] = time.Now().Sub(nslookup).Seconds()
	pinger.OnFinish = func(stats *ping.Statistics) {
		defaultICMPPorberResultReq.ProberSuccess = true
		defaultICMPPorberResultReq.ICMPFields["loss"] = stats.PacketLoss
		defaultICMPPorberResultReq.ICMPFields["rtt"] = stats.AvgRtt.Seconds()
		defaultICMPPorberResultReq.ICMPFields["stddev"] = stats.StdDevRtt.Seconds()
	}

	if err := pinger.Run(); err != nil {
		logrus.Errorln("ping run failed ", err)
	}
	return defaultICMPPorberResultReq
}
