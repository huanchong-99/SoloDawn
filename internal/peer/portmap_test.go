package peer

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huin/goupnp/dcps/internetgateway2"
	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/pion/ice/v4"
)

type fakeRoute struct {
	gw  net.IP
	err error
}

func (r fakeRoute) Route(net.IP) (*net.Interface, net.IP, net.IP, error) {
	return nil, r.gw, nil, r.err
}

type pmpCall struct {
	proto               string
	internal, requested int
	lifetime            int
}

type fakePMP struct {
	mu      sync.Mutex
	ip      [4]byte
	ipErr   error
	calls   []pmpCall
	handler func(pmpCall) (*natpmp.AddPortMappingResult, error)
}

func (f *fakePMP) GetExternalAddress() (*natpmp.GetExternalAddressResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ipErr != nil {
		return nil, f.ipErr
	}
	return &natpmp.GetExternalAddressResult{ExternalIPAddress: f.ip}, nil
}

func (f *fakePMP) AddPortMapping(proto string, internal, requested, lifetime int) (*natpmp.AddPortMappingResult, error) {
	c := pmpCall{proto: proto, internal: internal, requested: requested, lifetime: lifetime}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	h := f.handler
	f.mu.Unlock()
	if lifetime == 0 {
		return &natpmp.AddPortMappingResult{InternalPort: uint16(internal)}, nil
	}
	return h(c)
}

func (f *fakePMP) recorded() []pmpCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func silentIGDCfg() mapConfig {
	cfg := defaultMapConfig()
	cfg.openRoute = func() (routeReader, error) { return nil, errors.New("no default route") }
	cfg.localAddr = func(string) (netip.Addr, error) { return netip.MustParseAddr("10.0.0.2"), nil }
	cfg.findIP2 = func(context.Context) ([]*internetgateway2.WANIPConnection2, []error, error) { return nil, nil, nil }
	cfg.findIP1 = func(context.Context) ([]*internetgateway2.WANIPConnection1, []error, error) { return nil, nil, nil }
	cfg.findPPP1 = func(context.Context) ([]*internetgateway2.WANPPPConnection1, []error, error) { return nil, nil, nil }
	return cfg
}

type upnpAction struct {
	action, proto, client, desc string
	ext, internal               uint16
	lease                       uint32
}

type fakeUPnPServer struct {
	urn     string
	mu      sync.Mutex
	extIP   string
	ipErr   int
	actions []upnpAction
	add     func(upnpAction) int // 0 = ok, >0 = UPnP fault code, <0 = HTTP 500
	any     func(upnpAction) (uint16, int)
	hang    chan struct{}
}

func (s *fakeUPnPServer) start(t *testing.T) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL + "/root.xml")
	check(t, err)
	return u
}

func (s *fakeUPnPServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/root.xml" {
		fmt.Fprintf(w, `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><serviceList><service><serviceType>%s</serviceType><serviceId>id</serviceId><controlURL>/ctl</controlURL></service></serviceList></device></root>`, s.urn)
		return
	}
	s.mu.Lock()
	hang := s.hang
	s.mu.Unlock()
	if hang != nil {
		select {
		case <-hang:
		case <-r.Context().Done():
			return
		}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, name, _ := strings.Cut(strings.Trim(r.Header.Get("SOAPACTION"), `"`), "#")
	s.dispatchSOAP(w, name, string(body))
}

func xmlTag(body, tag string) string {
	_, rest, ok := strings.Cut(body, "<"+tag+">")
	if !ok {
		return ""
	}
	val, _, _ := strings.Cut(rest, "</"+tag+">")
	return val
}

func writeSOAPFault(w http.ResponseWriter, code int) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `%s<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError><errorCode>%d</errorCode><errorDescription>err</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, xml.Header, code)
}

func (s *fakeUPnPServer) dispatchSOAP(w http.ResponseWriter, name, body string) {
	ext, _ := strconv.Atoi(xmlTag(body, "NewExternalPort"))
	in, _ := strconv.Atoi(xmlTag(body, "NewInternalPort"))
	lease, _ := strconv.Atoi(xmlTag(body, "NewLeaseDuration"))
	act := upnpAction{
		action: name, proto: xmlTag(body, "NewProtocol"), client: xmlTag(body, "NewInternalClient"),
		desc: xmlTag(body, "NewPortMappingDescription"), ext: uint16(ext), internal: uint16(in), lease: uint32(lease),
	}
	s.mu.Lock()
	s.actions = append(s.actions, act)
	extIP, ipErr, addFn, anyFn := s.extIP, s.ipErr, s.add, s.any
	s.mu.Unlock()

	switch name {
	case "GetExternalIPAddress":
		if ipErr != 0 {
			writeSOAPFault(w, ipErr)
			return
		}
		fmt.Fprintf(w, `%s<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetExternalIPAddressResponse xmlns:u="%s"><NewExternalIPAddress>%s</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`, xml.Header, s.urn, extIP)
	case "AddPortMapping":
		code := 0
		if addFn != nil {
			code = addFn(act)
		}
		if code < 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if code > 0 {
			writeSOAPFault(w, code)
			return
		}
		fmt.Fprintf(w, `%s<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:AddPortMappingResponse xmlns:u="%s"/></s:Body></s:Envelope>`, xml.Header, s.urn)
	case "AddAnyPortMapping":
		port, code := uint16(0), 0
		if anyFn != nil {
			port, code = anyFn(act)
		}
		if code > 0 {
			writeSOAPFault(w, code)
			return
		}
		fmt.Fprintf(w, `%s<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:AddAnyPortMappingResponse xmlns:u="%s"><NewReservedPort>%d</NewReservedPort></u:AddAnyPortMappingResponse></s:Body></s:Envelope>`, xml.Header, s.urn, port)
	case "DeletePortMapping":
		fmt.Fprintf(w, `%s<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:DeletePortMappingResponse xmlns:u="%s"/></s:Body></s:Envelope>`, xml.Header, s.urn)
	}
}

func (s *fakeUPnPServer) recorded() []upnpAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.actions)
}

func TestDefaultMapConfigAndLocalHelpers(t *testing.T) {
	cfg := defaultMapConfig()
	eq(t, cfg.lease, uint32(7200))
	eq(t, cfg.timeout, 2*time.Second)
	eq(t, cfg.discoverTimeout, 3*time.Second)
	_, _ = cfg.openRoute()
	if cfg.newPMP(net.IPv4(127, 0, 0, 1), time.Second) == nil {
		t.Fatal("nil natpmp client")
	}
	ip, err := localIP("127.0.0.1")
	check(t, err)
	eq(t, ip, netip.MustParseAddr("127.0.0.1"))
	_, err = localIP("256.256.256.256")
	eq(t, err == nil, false)
	for _, bad := range []netip.Addr{{}, netip.IPv4Unspecified(), netip.MustParseAddr("::1")} {
		_, err = validIPv4(bad)
		eq(t, err == nil, false)
	}
	mapped4, err := validIPv4(netip.MustParseAddr("::ffff:198.51.100.9"))
	check(t, err)
	eq(t, mapped4, netip.MustParseAddr("198.51.100.9"))
	eq(t, halfLease(7200), time.Hour)
	eq(t, halfLease(4), 2*time.Second)
	eq(t, halfLease(3), 1500*time.Millisecond)
	eq(t, halfLease(2), time.Second)
	eq(t, halfLease(1), time.Second)
	eq(t, eligibleIP(net.ParseIP("10.0.0.2")), true)
	eq(t, eligibleIP(net.ParseIP("169.254.1.1")), true)
	eq(t, eligibleIP(net.ParseIP("2001:db8::1")), true)
	eq(t, eligibleIP(net.ParseIP("fe80::1")), false)
	eq(t, afterUsed(47800, nil), uint16(47801))
	eq(t, afterUsed(47800, map[uint16]bool{47801: true}), uint16(47802))
	eq(t, afterUsed(47800, map[uint16]bool{47805: true}), uint16(47806))
	eq(t, afterUsed(47805, map[uint16]bool{47801: true}), uint16(47806))
	sListen, err := Listen(0, 0)
	check(t, err)
	check(t, sListen.Close())

	base := published{mapped: true, signalURL: "ws://198.51.100.1:47800/rv", answerCands: []string{"a", "b"}, offerCands: []string{"b"}}
	eq(t, samePublished(base, base), true)
	for _, diff := range []published{
		{mapped: false, signalURL: base.signalURL, answerCands: base.answerCands, offerCands: base.offerCands},
		{mapped: true, signalURL: "ws://198.51.100.1:47801/rv", answerCands: base.answerCands, offerCands: base.offerCands},
		{mapped: true, signalURL: base.signalURL, answerCands: []string{"a"}, offerCands: base.offerCands},
		{mapped: true, signalURL: base.signalURL, answerCands: base.answerCands, offerCands: []string{"c"}},
	} {
		eq(t, samePublished(base, diff), false)
	}
}

// NAT-PMP wins over UPnP when it answers, publishes the granted ports (never
// the requested ones), renews at half the shortest granted lifetime suggesting
// the granted ports, republishes when a port changes, withdraws without a
// retry storm if renewal fails, and deletes with lifetime 0 on Close.
func TestNATPMPLifecycleRenewalAndClose(t *testing.T) {
	s := &Sockets{udp: emptyUDPMux(t, 47800), tcp: dummyTCPMux(t), tcpPort: 47801}
	synctest.Test(t, func(t *testing.T) {
		var igdCalled atomic.Int32
		var failRenew atomic.Bool
		var signalGrant atomic.Uint32
		signalGrant.Store(30100)

		pmp := &fakePMP{
			ip: [4]byte{198, 51, 100, 7},
			handler: func(c pmpCall) (*natpmp.AddPortMappingResult, error) {
				if failRenew.Load() {
					return nil, errors.New("gateway unreachable")
				}
				switch {
				case c.proto == "udp":
					return &natpmp.AddPortMappingResult{InternalPort: uint16(c.internal), MappedExternalPort: 30000, PortMappingLifetimeInSeconds: 120}, nil
				case c.internal == 47801:
					return &natpmp.AddPortMappingResult{InternalPort: 47801, MappedExternalPort: 30001, PortMappingLifetimeInSeconds: 80}, nil
				default:
					return &natpmp.AddPortMappingResult{InternalPort: 47800, MappedExternalPort: uint16(signalGrant.Load()), PortMappingLifetimeInSeconds: 100}, nil
				}
			},
		}
		cfg := silentIGDCfg()
		cfg.openRoute = func() (routeReader, error) { return fakeRoute{gw: net.IPv4(10, 0, 0, 1)}, nil }
		cfg.newPMP = func(gw net.IP, d time.Duration) pmpClient {
			eq(t, gw.String(), "10.0.0.1")
			eq(t, d, 2*time.Second)
			return pmp
		}
		cfg.findIP2 = func(ctx context.Context) ([]*internetgateway2.WANIPConnection2, []error, error) {
			igdCalled.Add(1)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}

		pm := s.mapPorts(47800, cfg)
		<-pm.Ready()
		synctest.Wait()

		eq(t, pm.Mapped(), true)
		eq(t, pm.SignalURL(), "ws://198.51.100.7:30100/rv")
		ansCands, ch1 := pm.candidates(true)
		offCands, _ := pm.candidates(false)
		eq(t, len(ansCands), 2)
		eq(t, strings.Contains(ansCands[0], " 198.51.100.7 30000 typ srflx raddr 10.0.0.2 rport 47800"), true)
		eq(t, strings.Contains(ansCands[1], " 198.51.100.7 30001 typ host tcptype passive"), true)
		eq(t, offCands, []string{ansCands[1]})

		// Shortest granted lifetime is 80 s, so renewal fires at 40 s exactly.
		time.Sleep(40*time.Second - time.Nanosecond)
		synctest.Wait()
		eq(t, len(pmp.recorded()), 3)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		calls := pmp.recorded()
		eq(t, len(calls), 6)
		eq(t, []int{calls[3].requested, calls[4].requested, calls[5].requested}, []int{30000, 30001, 30100})
		select {
		case <-ch1:
			t.Fatal("unchanged renewal closed changed channel")
		default:
		}

		// Next renewal changes the granted signalling port -> republished.
		signalGrant.Store(30200)
		time.Sleep(40 * time.Second)
		synctest.Wait()
		<-ch1
		eq(t, pm.SignalURL(), "ws://198.51.100.7:30200/rv")

		// Check Close deletes active mappings with lifetime 0 and withdraws.
		check(t, pm.Close())
		eq(t, pm.Mapped(), false)
		eq(t, pm.SignalURL(), "")
		candsAfter, _ := pm.candidates(true)
		eq(t, len(candsAfter), 0)
		calls = pmp.recorded()
		eq(t, calls[len(calls)-3:], []pmpCall{
			{proto: "udp", internal: 47800, requested: 0, lifetime: 0},
			{proto: "tcp", internal: 47801, requested: 0, lifetime: 0},
			{proto: "tcp", internal: 47800, requested: 0, lifetime: 0},
		})
		eq(t, igdCalled.Load(), int32(1))
	})
	check(t, s.Close())
}

// When renewal fails or no gateway exists, published state is withdrawn and
// no retry storm happens; Refresh re-runs discovery when the network changes.
func TestFailurePathNoRetryStormAndRefresh(t *testing.T) {
	s := &Sockets{udp: emptyUDPMux(t, 47800), tcp: dummyTCPMux(t), tcpPort: 47801}
	t.Cleanup(func() { s.Close() })
	synctest.Test(t, func(t *testing.T) {
		var allow atomic.Bool
		var attempts atomic.Int32
		pmp := &fakePMP{
			ip: [4]byte{198, 51, 100, 8},
			handler: func(c pmpCall) (*natpmp.AddPortMappingResult, error) {
				attempts.Add(1)
				if !allow.Load() {
					return nil, errors.New("refused")
				}
				return &natpmp.AddPortMappingResult{InternalPort: uint16(c.internal), MappedExternalPort: uint16(c.requested), PortMappingLifetimeInSeconds: 20}, nil
			},
		}
		cfg := silentIGDCfg()
		cfg.openRoute = func() (routeReader, error) { return fakeRoute{gw: net.IPv4(10, 0, 0, 1)}, nil }
		cfg.newPMP = func(net.IP, time.Duration) pmpClient { return pmp }

		pm := s.mapPorts(0, cfg)
		<-pm.Ready()
		synctest.Wait()
		eq(t, pm.Mapped(), false)
		eq(t, attempts.Load(), int32(2))

		// Advance 24 hours: zero retries while unmapped.
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		eq(t, attempts.Load(), int32(2))

		// Network change enables mapping; burst Refresh calls coalesce.
		allow.Store(true)
		pm.Refresh()
		pm.Refresh()
		pm.Refresh()
		synctest.Wait()
		eq(t, pm.Mapped(), true)
		eq(t, pm.SignalURL(), "")

		// Renewal at 10 s fails -> withdrawn immediately and no retries after.
		allow.Store(false)
		before := attempts.Load()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		eq(t, pm.Mapped(), false)
		afterFail := attempts.Load()
		if afterFail <= before {
			t.Fatal("expected renewal attempt")
		}
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		eq(t, attempts.Load(), afterFail)
		check(t, pm.Close())
	})
}

func TestUPnPIGDv2FallbackCollisionAndClose(t *testing.T) {
	// IGDv2: 47801 returns 606 -> AddAnyPortMapping grants 30001;
	// 47800/TCP returns 718 -> AddAnyPortMapping grants 47801;
	// if signalPort ran first and took 47801, used[47801] skips AddPortMapping.
	srv := &fakeUPnPServer{
		urn:   internetgateway2.URN_WANIPConnection_2,
		extIP: "203.0.113.5",
		add: func(a upnpAction) int {
			if a.proto == "UDP" {
				return upnpConflict
			}
			return upnpUnauthorized
		},
		any: func(a upnpAction) (uint16, int) {
			if a.proto == "UDP" {
				return 30500, 0
			}
			if a.internal == 47801 {
				return 47800, 0 // deliberately collides with signalPort's internal port!
			}
			return 30502, 0
		},
	}
	loc := srv.start(t)
	clients, err := internetgateway2.NewWANIPConnection2ClientsByURLCtx(t.Context(), loc)
	check(t, err)

	// Also register an IGDv1 client to verify IGDv2 takes priority over IGDv1.
	v1Srv := &fakeUPnPServer{urn: internetgateway2.URN_WANIPConnection_1, extIP: "192.0.2.99"}
	v1Clients, err := internetgateway2.NewWANIPConnection1ClientsByURLCtx(t.Context(), v1Srv.start(t))
	check(t, err)

	cfg := silentIGDCfg()
	cfg.findIP2 = func(context.Context) ([]*internetgateway2.WANIPConnection2, []error, error) { return clients, nil, nil }
	cfg.findIP1 = func(context.Context) ([]*internetgateway2.WANIPConnection1, []error, error) {
		return v1Clients, nil, nil
	}

	s := &Sockets{udp: emptyUDPMux(t, 47800), tcp: dummyTCPMux(t), tcpPort: 47801}
	pm := s.mapPorts(47800, cfg)
	t.Cleanup(func() { pm.Close() })
	<-pm.Ready()

	eq(t, pm.Mapped(), true)
	eq(t, pm.SignalURL(), "ws://203.0.113.5:30502/rv")
	eq(t, len(v1Srv.recorded()), 0)

	// Because tcpPort (47801) was granted external 47800, signalPort (47800) saw
	// used["tcp"][47800] == true and went straight to AddAnyPortMapping(47801)
	// without calling AddPortMapping(47800), avoiding miniupnpd's same-client overwrite.
	var sawSignalAddSpecific bool
	var anyStarts []uint16
	for _, a := range srv.recorded() {
		if a.action == "AddPortMapping" && a.proto == "TCP" && a.internal == 47800 {
			sawSignalAddSpecific = true
		}
		if a.action == "AddAnyPortMapping" {
			anyStarts = append(anyStarts, a.ext)
		}
	}
	eq(t, sawSignalAddSpecific, false)
	eq(t, anyStarts, []uint16{47801, 47802, 47801})

	check(t, pm.Close())
	eq(t, pm.Mapped(), false)
	eq(t, pm.SignalURL(), "")
	acts := srv.recorded()
	var deletes []uint16
	for _, a := range acts {
		if a.action == "DeletePortMapping" {
			deletes = append(deletes, a.ext)
		}
	}
	eq(t, deletes, []uint16{30500, 47800, 30502})
}

func TestUPnPIGDv1AndPPPFallbackAndErrors(t *testing.T) {
	// First WANIPConnection1 fails GetExternalIPAddress, so discovery falls
	// through to WANPPPConnection1, where conflicts on 47800 and 47801 step to
	// free adjacent ports without colliding with already-used TCP ports.
	badIP1 := &fakeUPnPServer{urn: internetgateway2.URN_WANIPConnection_1, ipErr: 501}
	ip1Clients, err := internetgateway2.NewWANIPConnection1ClientsByURLCtx(t.Context(), badIP1.start(t))
	check(t, err)

	pppSrv := &fakeUPnPServer{
		urn:   internetgateway2.URN_WANPPPConnection_1,
		extIP: "203.0.113.9",
		add: func(a upnpAction) int {
			if a.proto == "UDP" && a.ext == 47800 {
				return upnpConflict // 47801 succeeds on step 1
			}
			if a.proto == "TCP" && a.internal == 47801 && a.ext == 47801 {
				return 0 // takes TCP 47801
			}
			if a.proto == "TCP" && a.internal == 47800 {
				// 47800 returns 718; 47801 is in used["tcp"] so fallback starts at 47802:
				// step 0 (47802) and step 1 (47803) return 718, step 2 (47804) succeeds!
				if a.ext == 47800 || a.ext == 47802 || a.ext == 47803 {
					return upnpConflict
				}
				return 0
			}
			return 0
		},
	}
	pppClients, err := internetgateway2.NewWANPPPConnection1ClientsByURLCtx(t.Context(), pppSrv.start(t))
	check(t, err)

	cfg := silentIGDCfg()
	cfg.findIP1 = func(context.Context) ([]*internetgateway2.WANIPConnection1, []error, error) {
		return ip1Clients, nil, nil
	}
	cfg.findPPP1 = func(context.Context) ([]*internetgateway2.WANPPPConnection1, []error, error) {
		return pppClients, nil, nil
	}

	s := &Sockets{udp: emptyUDPMux(t, 47800), tcp: dummyTCPMux(t), tcpPort: 47801}
	pm := s.mapPorts(47800, cfg)
	<-pm.Ready()
	eq(t, pm.Mapped(), true)
	eq(t, pm.SignalURL(), "ws://203.0.113.9:47804/rv")
	check(t, pm.Close())
}

type errAnyPort struct{}

func (errAnyPort) AddAnyPortMappingCtx(context.Context, string, uint16, string, uint16, string, bool, string, uint32) (uint16, error) {
	return 30000, errors.New("soap error")
}

func TestGatewayBoundaryRejectionsAndTimeouts(t *testing.T) {
	// Route & gateway rejections in newPMPGateway.
	for _, r := range []struct {
		open func() (routeReader, error)
	}{
		{func() (routeReader, error) { return nil, errors.New("netlink failed") }},
		{func() (routeReader, error) { return fakeRoute{err: errors.New("no route")}, nil }},
		{func() (routeReader, error) { return fakeRoute{gw: nil}, nil }},
		{func() (routeReader, error) { return fakeRoute{gw: net.IPv4zero}, nil }},
		{func() (routeReader, error) { return fakeRoute{gw: net.ParseIP("fe80::1")}, nil }},
	} {
		cfg := silentIGDCfg()
		cfg.openRoute = r.open
		pm := &PortMap{cfg: cfg}
		eq(t, pm.newPMPGateway(), nil)
	}

	// NAT-PMP rejections: localAddr error, GetExternalAddress error, 0.0.0.0 external IP,
	// port 0 / lifetime 0, and nextSuggested skipping used ports.
	pmp := &fakePMP{
		ip: [4]byte{198, 51, 100, 1},
		handler: func(c pmpCall) (*natpmp.AddPortMappingResult, error) {
			if c.internal == 1 {
				return &natpmp.AddPortMappingResult{MappedExternalPort: 0, PortMappingLifetimeInSeconds: 60}, nil
			}
			if c.internal == 2 {
				return &natpmp.AddPortMappingResult{MappedExternalPort: 5000, PortMappingLifetimeInSeconds: 0}, nil
			}
			return &natpmp.AddPortMappingResult{MappedExternalPort: uint16(c.requested), PortMappingLifetimeInSeconds: 60}, nil
		},
	}
	g := &pmpGateway{host: "10.0.0.1", localAddr: func(string) (netip.Addr, error) { return netip.Addr{}, errors.New("no iface") }, client: pmp}
	_, _, err := g.addrs(t.Context())
	eq(t, err == nil, false)
	g.localAddr = func(string) (netip.Addr, error) { return netip.MustParseAddr("10.0.0.2"), nil }
	pmp.ipErr = errors.New("timeout")
	_, _, err = g.addrs(t.Context())
	eq(t, err == nil, false)
	pmp.ipErr = nil
	pmp.ip = [4]byte{0, 0, 0, 0}
	_, _, err = g.addrs(t.Context())
	eq(t, err == nil, false)
	pmp.ip = [4]byte{198, 51, 100, 1}
	_, _, err = g.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 1, 1, map[uint16]bool{}, 60)
	eq(t, err == nil, false)
	_, _, err = g.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 2, 2, map[uint16]bool{}, 60)
	eq(t, err == nil, false)
	port, _, err := g.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "tcp", 47800, 47800, map[uint16]bool{47800: true, 47801: true}, 60)
	check(t, err)
	eq(t, port, uint16(47802))

	// UPnP rejections: localAddr error, invalid external IP, non-718/606 SOAP fault,
	// HTTP 500 non-SOAP error, AddAnyPortMapping returning port 0, IGDv1 exhausting all 3 steps,
	// and SOAP call timeout when gateway hangs.
	srv := &fakeUPnPServer{
		urn:   internetgateway2.URN_WANIPConnection_2,
		extIP: "not-an-ip",
	}
	loc := srv.start(t)
	c2, err := internetgateway2.NewWANIPConnection2ClientsByURLCtx(t.Context(), loc)
	check(t, err)

	cfg := silentIGDCfg()
	cfg.timeout = 40 * time.Millisecond
	ig := newIGDGateway(&c2[0].ServiceClient, c2[0], c2[0], cfg).(*igdGateway)
	_, _, err = ig.addrs(t.Context())
	eq(t, err == nil, false)
	srv.extIP = "0.0.0.0"
	_, _, err = ig.addrs(t.Context())
	eq(t, err == nil, false)
	ig.localAddr = func(string) (netip.Addr, error) { return netip.Addr{}, errors.New("no local") }
	_, _, err = ig.addrs(t.Context())
	eq(t, err == nil, false)
	ig.localAddr = cfg.localAddr
	srv.extIP = "203.0.113.10"

	// Non-conflict SOAP fault (501) and non-SOAP HTTP 500 do not fall back to AddAnyPortMapping.
	srv.add = func(upnpAction) int { return 501 }
	_, _, err = ig.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 47800, 47800, map[uint16]bool{}, 60)
	eq(t, err == nil, false)
	srv.add = func(upnpAction) int { return -1 }
	_, _, err = ig.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 47800, 47800, map[uint16]bool{}, 60)
	eq(t, err == nil, false)

	// AddAnyPortMapping returning port 0 or a non-zero port with an error both fail.
	srv.add = func(upnpAction) int { return upnpConflict }
	srv.any = func(upnpAction) (uint16, int) { return 0, 0 }
	_, _, err = ig.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 47800, 47800, map[uint16]bool{}, 60)
	eq(t, err == nil, false)
	igErrAny := newIGDGateway(&c2[0].ServiceClient, c2[0], errAnyPort{}, cfg).(*igdGateway)
	_, _, err = igErrAny.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 47800, 47800, map[uint16]bool{}, 60)
	eq(t, err == nil, false)

	// IGDv1 exhausting all 3 steps (47801..47803 fail; 47804 would succeed if tried).
	srv.add = func(a upnpAction) int {
		if a.ext <= 47803 {
			return upnpConflict
		}
		return 0
	}
	igNoAny := newIGDGateway(&c2[0].ServiceClient, c2[0], nil, cfg).(*igdGateway)
	_, _, err = igNoAny.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "udp", 47800, 47800, map[uint16]bool{}, 60)
	eq(t, err == nil, false)
	// Also verify AddAnyPortMapping starts past used={47801:true} (at 47802) when 47800 conflicts.
	var gotAnyExt uint16
	srv.any = func(a upnpAction) (uint16, int) { gotAnyExt = a.ext; return a.ext, 0 }
	gotPort, _, err := ig.mapPort(t.Context(), netip.MustParseAddr("10.0.0.2"), "tcp", 47800, 47800, map[uint16]bool{47801: true}, 60)
	check(t, err)
	eq(t, gotAnyExt, uint16(47802))
	eq(t, gotPort, uint16(47802))

	// Hanging UPnP server is bounded by cfg.timeout.
	hang := make(chan struct{})
	defer close(hang)
	srv.mu.Lock()
	srv.hang = hang
	srv.mu.Unlock()
	start := time.Now()
	_, _, err = ig.addrs(t.Context())
	eq(t, err == nil, false)
	if time.Since(start) > time.Second {
		t.Fatalf("gateway call ignored timeout: %v", time.Since(start))
	}
}

func TestZeroTargetsAndCloseDuringDiscovery(t *testing.T) {
	// Zero listen addresses on UDP mux + zero tcpPort + zero signalPort -> empty targets;
	// discovery (openRoute) must not even run.
	udp, err := ice.NewMultiUDPMuxFromPort(0, ice.UDPMuxFromPortWithIPFilter(func(net.IP) bool { return false }))
	check(t, err)
	s := &Sockets{udp: udp, tcp: dummyTCPMux(t), tcpPort: 0}
	eq(t, s.udpListenPort(), 0)
	emptyCfg := silentIGDCfg()
	emptyCfg.openRoute = func() (routeReader, error) {
		t.Fatal("openRoute called with zero targets")
		return nil, nil
	}
	pm := s.mapPorts(0, emptyCfg)
	<-pm.Ready()
	eq(t, pm.Mapped(), false)
	check(t, s.Close())

	// Sockets.Close() while PortMap is actively Mapped() == true withdraws and deletes.
	pmp4 := &fakePMP{
		ip: [4]byte{198, 51, 100, 55},
		handler: func(c pmpCall) (*natpmp.AddPortMappingResult, error) {
			return &natpmp.AddPortMappingResult{InternalPort: uint16(c.internal), MappedExternalPort: uint16(c.requested), PortMappingLifetimeInSeconds: 600}, nil
		},
	}
	cfg4 := silentIGDCfg()
	cfg4.openRoute = func() (routeReader, error) { return fakeRoute{gw: net.IPv4(10, 0, 0, 1)}, nil }
	cfg4.newPMP = func(net.IP, time.Duration) pmpClient { return pmp4 }
	s4 := &Sockets{udp: emptyUDPMux(t, 47800), tcp: dummyTCPMux(t), tcpPort: 47801}
	pm4 := s4.mapPorts(0, cfg4)
	<-pm4.Ready()
	eq(t, pm4.Mapped(), true)
	check(t, s4.Close())
	eq(t, pm4.Mapped(), false)
	calls4 := pmp4.recorded()
	eq(t, calls4[len(calls4)-2:], []pmpCall{
		{proto: "udp", internal: 47800, requested: 0, lifetime: 0},
		{proto: "tcp", internal: 47801, requested: 0, lifetime: 0},
	})

	// Close while IGD discovery is blocked hits awaitIGD's ctx.Done() and still closes Ready.
	blocking := silentIGDCfg()
	started := make(chan struct{})
	blocking.findIP2 = func(ctx context.Context) ([]*internetgateway2.WANIPConnection2, []error, error) {
		close(started)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	s2 := sockets(t)
	pm2 := s2.mapPorts(47800, blocking)
	<-started
	check(t, pm2.Close())
	<-pm2.Ready()
	eq(t, pm2.Mapped(), false)

	// Real MapPorts entrypoint closed immediately does not leak.
	s3 := sockets(t)
	pm3 := s3.MapPorts(0)
	check(t, pm3.Close())
}

// Peer trickles mapped UDP (srflx) + passive ICE-TCP (host) on answered
// connections, and ONLY mapped passive ICE-TCP (never UDP) on offered
// connections, including when PortMap finishes or updates while the connection
// is already open; closing PortMap ends the per-connection watcher cleanly.
func TestPeerPublishesMappedCandidatesPerRoleAndLateArrival(t *testing.T) {
	f := newPair(t)
	var allow atomic.Bool
	var udpExt atomic.Uint32
	udpExt.Store(31000)

	pmp := &fakePMP{
		ip: [4]byte{198, 51, 100, 42},
		handler: func(c pmpCall) (*natpmp.AddPortMappingResult, error) {
			if !allow.Load() {
				return nil, errors.New("not mapped yet")
			}
			if c.proto == "udp" {
				return &natpmp.AddPortMappingResult{InternalPort: uint16(c.internal), MappedExternalPort: uint16(udpExt.Load()), PortMappingLifetimeInSeconds: 3600}, nil
			}
			return &natpmp.AddPortMappingResult{InternalPort: uint16(c.internal), MappedExternalPort: 31001, PortMappingLifetimeInSeconds: 3600}, nil
		},
	}
	cfg := silentIGDCfg()
	cfg.openRoute = func() (routeReader, error) { return fakeRoute{gw: net.IPv4(10, 0, 0, 1)}, nil }
	cfg.newPMP = func(net.IP, time.Duration) pmpClient { return pmp }

	// Attach PortMap to both f.a (offerer) and f.b (answerer), initially unmapped.
	pmA := f.a.sockets.mapPorts(0, cfg)
	pmB := f.b.sockets.mapPorts(47800, cfg)
	<-pmA.Ready()
	<-pmB.Ready()

	// Open a -> b while unmapped; neither side has sent a mapped candidate yet.
	n := f.open(t, 0)
	n.info(t)

	// Now enable mappings and Refresh: both live connections trickle their role's
	// mapped candidates without blocking!
	allow.Store(true)
	pmA.Refresh()
	pmB.Refresh()

	collectMapped := func(m mailbox, want int) []string {
		var got []string
		deadline := time.After(5 * time.Second)
		for len(got) < want {
			select {
			case env := <-m["ice"]:
				var c proto.ICECandidate
				check(t, proto.Decode([]byte(env.Payload), &c))
				if strings.Contains(c.Candidate, "198.51.100.42") {
					got = append(got, c.Candidate)
				}
			case <-deadline:
				t.Fatalf("got %v, want %d mapped candidates", got, want)
			}
		}
		return got
	}

	// Offerer f.a sends ONLY passive TCP (1 candidate), NEVER srflx UDP!
	aCands := collectMapped(f.aMail, 1)
	eq(t, strings.Contains(aCands[0], " tcp ") && strings.Contains(aCands[0], "198.51.100.42 31001 typ host tcptype passive"), true)

	// Answerer f.b sends BOTH srflx UDP and passive TCP (2 candidates).
	bCands := collectMapped(f.bMail, 2)
	eq(t, strings.Contains(bCands[0], " udp ") && strings.Contains(bCands[0], "198.51.100.42 31000 typ srflx raddr 10.0.0.2"), true)
	eq(t, strings.Contains(bCands[1], " tcp ") && strings.Contains(bCands[1], "198.51.100.42 31001 typ host tcptype passive"), true)

	// Verify f.a sent no extra mapped UDP candidate.
	for len(f.aMail["ice"]) > 0 {
		env := <-f.aMail["ice"]
		if strings.Contains(env.Payload, "198.51.100.42") {
			t.Fatalf("offerer published unexpected mapped candidate: %s", env.Payload)
		}
	}

	// Closing pmA stops f.a's trickleMapped via <-pm.done; closing n.server stops f.b's via <-c.Done().
	check(t, pmA.Close())
	check(t, n.server.Close())
	check(t, n.client.Close())
	check(t, pmB.Close())
}

func emptyUDPMux(t *testing.T, port int) ice.UDPMux {
	t.Helper()
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopbackIP, Port: 0})
	check(t, err)
	t.Cleanup(func() { l.Close() })
	return fixedPortUDPMux{UDPMux: ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: l}), port: port}
}

type fixedPortUDPMux struct {
	ice.UDPMux
	port int
}

func (m fixedPortUDPMux) GetListenAddresses() []net.Addr {
	return []net.Addr{&net.UDPAddr{IP: loopbackIP, Port: m.port}}
}

func dummyTCPMux(t *testing.T) ice.TCPMux {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	check(t, err)
	mux := ice.NewTCPMuxDefault(ice.TCPMuxParams{Listener: l, ReadBufferSize: 8192})
	t.Cleanup(func() { mux.Close() })
	return mux
}
