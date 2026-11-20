package peer

import (
	"cmp"
	"context"
	"errors"
	"math"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/huin/goupnp/soap"
	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/libp2p/go-netroute"
	"github.com/pion/ice/v4"
)

const (
	defaultLease     = 7200
	defaultTimeout   = 2 * time.Second
	discoverDeadline = 3 * time.Second
	upnpConflict     = 718
	upnpUnauthorized = 606
)

type pmpClient interface {
	GetExternalAddress() (*natpmp.GetExternalAddressResult, error)
	AddPortMapping(protocol string, internalPort, requestedExternalPort, lifetime int) (*natpmp.AddPortMappingResult, error)
}

type routeReader interface {
	Route(dst net.IP) (*net.Interface, net.IP, net.IP, error)
}

type igdClient interface {
	GetExternalIPAddressCtx(ctx context.Context) (string, error)
	AddPortMappingCtx(ctx context.Context, remoteHost string, extPort uint16, proto string, intPort uint16, client string, enabled bool, desc string, lease uint32) error
	DeletePortMappingCtx(ctx context.Context, remoteHost string, extPort uint16, proto string) error
}

type anyPortClient interface {
	AddAnyPortMappingCtx(ctx context.Context, remoteHost string, extPort uint16, proto string, intPort uint16, client string, enabled bool, desc string, lease uint32) (uint16, error)
}

type gateway interface {
	addrs(ctx context.Context) (local, external netip.Addr, err error)
	mapPort(ctx context.Context, local netip.Addr, proto string, internal, suggested int, used map[uint16]bool, lease uint32) (uint16, uint32, error)
	deletePort(ctx context.Context, proto string, internal int, external uint16)
}

type mapConfig struct {
	lease           uint32
	timeout         time.Duration
	discoverTimeout time.Duration
	openRoute       func() (routeReader, error)
	newPMP          func(net.IP, time.Duration) pmpClient
	localAddr       func(string) (netip.Addr, error)
	findIP2         func(context.Context) ([]*internetgateway2.WANIPConnection2, []error, error)
	findIP1         func(context.Context) ([]*internetgateway2.WANIPConnection1, []error, error)
	findPPP1        func(context.Context) ([]*internetgateway2.WANPPPConnection1, []error, error)
}

func defaultMapConfig() mapConfig {
	return mapConfig{
		lease:           defaultLease,
		timeout:         defaultTimeout,
		discoverTimeout: discoverDeadline,
		openRoute:       func() (routeReader, error) { return netroute.New() },
		newPMP:          func(gw net.IP, d time.Duration) pmpClient { return natpmp.NewClientWithTimeout(gw, d) },
		localAddr:       localIP,
		findIP2:         internetgateway2.NewWANIPConnection2ClientsCtx,
		findIP1:         internetgateway2.NewWANIPConnection1ClientsCtx,
		findPPP1:        internetgateway2.NewWANPPPConnection1ClientsCtx,
	}
}

type portKind int

const (
	kindUDP portKind = iota
	kindTCP
	kindSignal
)

type portTarget struct {
	kind     portKind
	proto    string
	internal int
}

type mappedEntry struct {
	portTarget
	external uint16
}

type published struct {
	mapped      bool
	signalURL   string
	answerCands []string
	offerCands  []string
}

// PortMap maintains NAT-PMP or UPnP IGD mappings for a node's answering UDP
// socket, passive ICE-TCP listener and TCP signalling listener, renewing them
// at half their granted lifetime and deleting them on Close
// (contracts/rendezvous.md sections 5 and 5.1).
type PortMap struct {
	cfg     mapConfig
	targets []portTarget
	ctx     context.Context
	cancel  context.CancelFunc
	stop    chan struct{}
	done    chan struct{}
	ready   chan struct{}
	refresh chan struct{}
	close   func() error

	mu      sync.Mutex
	pub     published
	changed chan struct{}

	gw      gateway
	entries []mappedEntry
}

// MapPorts starts background port mapping for s's answering UDP and passive
// ICE-TCP sockets, and for signalPort when positive. No connection waits for
// it: granted endpoints are published once mapped and withdrawn on Close or
// failed renewal.
func (s *Sockets) MapPorts(signalPort int) *PortMap {
	return s.mapPorts(signalPort, defaultMapConfig())
}

func (s *Sockets) mapPorts(signalPort int, cfg mapConfig) *PortMap {
	ctx, cancel := context.WithCancel(context.Background())
	pm := &PortMap{
		cfg:     cfg,
		targets: buildTargets(s.udpListenPort(), s.tcpPort, signalPort),
		ctx:     ctx,
		cancel:  cancel,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
		refresh: make(chan struct{}, 1),
		changed: make(chan struct{}),
	}
	pm.close = sync.OnceValue(func() error {
		cancel()
		close(pm.stop)
		<-pm.done
		return nil
	})
	s.portmap.Store(pm)
	go pm.loop()
	return pm
}

func (s *Sockets) closePortMap() error {
	if pm := s.portmap.Swap(nil); pm != nil {
		return pm.Close()
	}
	return nil
}

func (s *Sockets) udpListenPort() int {
	addrs := s.udp.GetListenAddresses()
	if len(addrs) == 0 {
		return 0
	}
	return addrs[0].(*net.UDPAddr).Port
}

func buildTargets(udpPort, tcpPort, signalPort int) []portTarget {
	var out []portTarget
	for _, t := range []portTarget{
		{kindUDP, "udp", udpPort},
		{kindTCP, "tcp", tcpPort},
		{kindSignal, "tcp", signalPort},
	} {
		if t.internal > 0 {
			out = append(out, t)
		}
	}
	return out
}

// Ready is closed once the initial discovery and mapping attempt finishes,
// whether or not a gateway was found.
func (pm *PortMap) Ready() <-chan struct{} { return pm.ready }

// Mapped reports whether at least one port mapping is currently published.
func (pm *PortMap) Mapped() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.pub.mapped
}

// SignalURL returns the external ws://<ip>:<port>/rv URL for the mapped TCP
// signalling port, or "" when no signalling port mapping is active.
func (pm *PortMap) SignalURL() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.pub.signalURL
}

// Refresh schedules a new gateway discovery and port-mapping pass after a
// network change, without blocking.
func (pm *PortMap) Refresh() {
	select {
	case pm.refresh <- struct{}{}:
		return
	default:
		return
	}
}

// Close withdraws published endpoints, deletes active mappings on the gateway
// and stops the renewal loop.
func (pm *PortMap) Close() error { return pm.close() }

func (pm *PortMap) candidates(answer bool) ([]string, <-chan struct{}) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if answer {
		return pm.pub.answerCands, pm.changed
	}
	return pm.pub.offerCands, pm.changed
}

func (p *Peer) startMapped(k link, c *Connection, mid string, answer bool) {
	if pm := p.sockets.portmap.Load(); pm != nil {
		go p.trickleMapped(pm, k, c, mid, answer)
	}
}

func (p *Peer) trickleMapped(pm *PortMap, k link, c *Connection, mid string, answer bool) {
	sent := make(map[string]bool)
	for {
		cands, changed := pm.candidates(answer)
		p.sendMapped(k, c, mid, cands, sent)
		select {
		case <-c.Done():
			return
		case <-pm.done:
			return
		case <-changed:
			continue
		}
	}
}

func (p *Peer) sendMapped(k link, c *Connection, mid string, cands []string, sent map[string]bool) {
	for _, cand := range cands {
		if !sent[cand] {
			sent[cand] = true
			msg := proto.ICECandidate{SignalHeader: p.header(k, "ice"), Candidate: cand, MID: mid}
			_ = broadcast(c.ctx, p.paths(k.device), k.device, p.sign(msg))
		}
	}
}

func (pm *PortMap) loop() {
	defer close(pm.done)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	pm.remap(timer)
	close(pm.ready)
	for {
		select {
		case <-pm.stop:
			pm.shutdown()
			return
		case <-pm.refresh:
			pm.remap(timer)
		case <-timer.C:
			pm.renew(timer)
		}
	}
}

func (pm *PortMap) remap(timer *time.Timer) {
	timer.Stop()
	if len(pm.targets) == 0 {
		pm.setPublished(published{})
		return
	}
	gw, entries, pub, lease, ok := pm.discover(pm.ctx)
	if !ok {
		pm.gw, pm.entries = nil, nil
		pm.setPublished(published{})
		return
	}
	pm.gw, pm.entries = gw, entries
	pm.setPublished(pub)
	timer.Reset(halfLease(lease))
}

func (pm *PortMap) renew(timer *time.Timer) {
	entries, pub, lease, ok := pm.apply(pm.ctx, pm.gw, pm.entries)
	if !ok {
		pm.gw, pm.entries = nil, nil
		pm.setPublished(published{})
		return
	}
	pm.entries = entries
	pm.setPublished(pub)
	timer.Reset(halfLease(lease))
}

func (pm *PortMap) shutdown() {
	pm.setPublished(published{})
	ctx, cancel := context.WithTimeout(context.Background(), pm.cfg.timeout)
	defer cancel()
	for _, e := range pm.entries {
		pm.gw.deletePort(ctx, e.proto, e.internal, e.external)
	}
	pm.gw, pm.entries = nil, nil
}

func halfLease(seconds uint32) time.Duration {
	return max(time.Duration(seconds)*time.Second/2, time.Second)
}

func (pm *PortMap) setPublished(next published) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if samePublished(pm.pub, next) {
		return
	}
	pm.pub = next
	close(pm.changed)
	pm.changed = make(chan struct{})
}

func samePublished(a, b published) bool {
	return a.mapped == b.mapped && a.signalURL == b.signalURL &&
		slices.Equal(a.answerCands, b.answerCands) && slices.Equal(a.offerCands, b.offerCands)
}

// discover starts UPnP IGD discovery alongside a NAT-PMP attempt on the
// default gateway, using NAT-PMP when it answers and falling back to IGDv2
// then IGDv1.
func (pm *PortMap) discover(ctx context.Context) (gateway, []mappedEntry, published, uint32, bool) {
	igdCtx, cancelIGD := context.WithCancel(ctx)
	defer cancelIGD()
	igdCh := make(chan []gateway, 1)
	go func() { igdCh <- pm.discoverIGD(igdCtx) }()
	if gw := pm.newPMPGateway(); gw != nil {
		if entries, pub, lease, ok := pm.apply(ctx, gw, nil); ok {
			return gw, entries, pub, lease, true
		}
	}
	return pm.awaitIGD(ctx, igdCh)
}

func (pm *PortMap) awaitIGD(ctx context.Context, igdCh <-chan []gateway) (gateway, []mappedEntry, published, uint32, bool) {
	select {
	case <-ctx.Done():
		return nil, nil, published{}, 0, false
	case igds := <-igdCh:
		for _, gw := range igds {
			if entries, pub, lease, ok := pm.apply(ctx, gw, nil); ok {
				return gw, entries, pub, lease, true
			}
		}
		return nil, nil, published{}, 0, false
	}
}

func (pm *PortMap) discoverIGD(ctx context.Context) []gateway {
	ctx, cancel := context.WithTimeout(ctx, pm.cfg.discoverTimeout)
	defer cancel()
	var (
		ip2  []*internetgateway2.WANIPConnection2
		ip1  []*internetgateway2.WANIPConnection1
		ppp1 []*internetgateway2.WANPPPConnection1
		wg   sync.WaitGroup
	)
	wg.Add(3)
	go func() { defer wg.Done(); ip2, _, _ = pm.cfg.findIP2(ctx) }()
	go func() { defer wg.Done(); ip1, _, _ = pm.cfg.findIP1(ctx) }()
	go func() { defer wg.Done(); ppp1, _, _ = pm.cfg.findPPP1(ctx) }()
	wg.Wait()
	var out []gateway
	for _, c := range ip2 {
		out = append(out, newIGDGateway(&c.ServiceClient, c, c, pm.cfg))
	}
	for _, c := range ip1 {
		out = append(out, newIGDGateway(&c.ServiceClient, c, nil, pm.cfg))
	}
	for _, c := range ppp1 {
		out = append(out, newIGDGateway(&c.ServiceClient, c, nil, pm.cfg))
	}
	return out
}

func (pm *PortMap) apply(ctx context.Context, gw gateway, prev []mappedEntry) ([]mappedEntry, published, uint32, bool) {
	local, external, err := gw.addrs(ctx)
	if err != nil {
		return nil, published{}, 0, false
	}
	used := map[string]map[uint16]bool{"udp": {}, "tcp": {}}
	var entries []mappedEntry
	minLease := uint32(math.MaxUint32)
	for _, t := range pm.targets {
		ext, lease, err := gw.mapPort(ctx, local, t.proto, t.internal, suggestedPort(t.kind, t.internal, prev), used[t.proto], pm.cfg.lease)
		if err != nil {
			continue
		}
		used[t.proto][ext] = true
		entries = append(entries, mappedEntry{t, ext})
		minLease = min(minLease, lease)
	}
	if len(entries) == 0 {
		return nil, published{}, 0, false
	}
	return entries, buildPublished(local, external, entries), minLease, true
}

func suggestedPort(kind portKind, fallback int, prev []mappedEntry) int {
	for _, e := range prev {
		if e.kind == kind {
			return int(e.external)
		}
	}
	return fallback
}

func buildPublished(local, external netip.Addr, entries []mappedEntry) published {
	pub := published{mapped: true}
	for _, e := range entries {
		switch e.kind {
		case kindUDP:
			pub.answerCands = append(pub.answerCands, udpCandidate(local, external, e.internal, e.external))
		case kindTCP:
			c := tcpCandidate(external, e.external)
			pub.answerCands = append(pub.answerCands, c)
			pub.offerCands = append(pub.offerCands, c)
		case kindSignal:
			pub.signalURL = "ws://" + netip.AddrPortFrom(external, e.external).String() + "/rv"
		}
	}
	return pub
}

func udpCandidate(local, external netip.Addr, internal int, port uint16) string {
	c, err := ice.NewCandidateServerReflexive(&ice.CandidateServerReflexiveConfig{
		Network: "udp", Address: external.String(), Port: int(port), Component: 1,
		RelAddr: local.String(), RelPort: internal,
	})
	must(err)
	return "candidate:" + c.Marshal()
}

func tcpCandidate(external netip.Addr, port uint16) string {
	c, err := ice.NewCandidateHost(&ice.CandidateHostConfig{
		Network: "tcp", Address: external.String(), Port: int(port), Component: 1,
		TCPType: ice.TCPTypePassive,
	})
	must(err)
	return "candidate:" + c.Marshal()
}

func validIPv4(ip netip.Addr) (netip.Addr, error) {
	ip = ip.Unmap()
	if !ip.Is4() || ip.IsUnspecified() {
		return netip.Addr{}, errors.New("invalid IPv4 address")
	}
	return ip, nil
}

func localIP(host string) (netip.Addr, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(host, "5351"))
	if err != nil {
		return netip.Addr{}, err
	}
	defer c.Close()
	return validIPv4(c.LocalAddr().(*net.UDPAddr).AddrPort().Addr())
}

func (pm *PortMap) newPMPGateway() gateway {
	r, err := pm.cfg.openRoute()
	if err != nil {
		return nil
	}
	_, gw, _, err := r.Route(net.IPv4zero)
	if err != nil {
		return nil
	}
	addr, ok := netip.AddrFromSlice(gw)
	if !ok {
		return nil
	}
	ip, err := validIPv4(addr)
	if err != nil {
		return nil
	}
	return &pmpGateway{host: ip.String(), localAddr: pm.cfg.localAddr, client: pm.cfg.newPMP(net.IP(ip.AsSlice()), pm.cfg.timeout)}
}

type pmpGateway struct {
	host      string
	localAddr func(string) (netip.Addr, error)
	client    pmpClient
}

func (g *pmpGateway) addrs(context.Context) (netip.Addr, netip.Addr, error) {
	local, err := g.localAddr(g.host)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	res, err := g.client.GetExternalAddress()
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	ext, err := validIPv4(netip.AddrFrom4(res.ExternalIPAddress))
	return local, ext, err
}

func (g *pmpGateway) mapPort(_ context.Context, _ netip.Addr, proto string, internal, suggested int, used map[uint16]bool, lease uint32) (uint16, uint32, error) {
	suggested = nextSuggested(suggested, used)
	res, err := g.client.AddPortMapping(proto, internal, suggested, int(lease))
	if err != nil {
		return 0, 0, err
	}
	if res.MappedExternalPort == 0 || res.PortMappingLifetimeInSeconds == 0 {
		return 0, 0, errors.New("empty NAT-PMP lease")
	}
	return res.MappedExternalPort, res.PortMappingLifetimeInSeconds, nil
}

func (g *pmpGateway) deletePort(_ context.Context, proto string, internal int, _ uint16) {
	_, _ = g.client.AddPortMapping(proto, internal, 0, 0)
}

func nextSuggested(suggested int, used map[uint16]bool) int {
	for used[uint16(suggested)] {
		suggested++
	}
	return suggested
}

type igdGateway struct {
	host      string
	localAddr func(string) (netip.Addr, error)
	client    igdClient
	any       anyPortClient
	timeout   time.Duration
}

func newIGDGateway(sc *goupnp.ServiceClient, client igdClient, any anyPortClient, cfg mapConfig) gateway {
	return &igdGateway{host: sc.SOAPClient.EndpointURL.Hostname(), localAddr: cfg.localAddr, client: client, any: any, timeout: cfg.timeout}
}

func (g *igdGateway) addrs(ctx context.Context) (netip.Addr, netip.Addr, error) {
	local, err := g.localAddr(g.host)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	raw, err := g.client.GetExternalIPAddressCtx(callCtx)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	parsed, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	ext, err := validIPv4(parsed)
	return local, ext, err
}

func (g *igdGateway) mapPort(ctx context.Context, local netip.Addr, proto string, internal, suggested int, used map[uint16]bool, lease uint32) (uint16, uint32, error) {
	upProto, client := strings.ToUpper(proto), local.String()
	if !used[uint16(suggested)] {
		err := g.addSpecific(ctx, uint16(suggested), upProto, uint16(internal), client, lease)
		if err == nil {
			return uint16(suggested), lease, nil
		}
		if !isPortRefused(err) {
			return 0, 0, err
		}
	}
	return g.fallbackPort(ctx, uint16(suggested), upProto, uint16(internal), client, used, lease)
}

func (g *igdGateway) fallbackPort(ctx context.Context, suggested uint16, proto string, internal uint16, client string, used map[uint16]bool, lease uint32) (uint16, uint32, error) {
	if g.any != nil {
		callCtx, cancel := context.WithTimeout(ctx, g.timeout)
		defer cancel()
		port, err := g.any.AddAnyPortMappingCtx(callCtx, "", suggested, proto, internal, client, true, "SoloDawn", lease)
		if err != nil || port == 0 {
			return 0, 0, cmp.Or(err, errors.New("empty UPnP reserved port"))
		}
		return port, lease, nil
	}
	return g.tryNextPorts(ctx, proto, internal, client, used, lease)
}

func (g *igdGateway) tryNextPorts(ctx context.Context, proto string, internal uint16, client string, used map[uint16]bool, lease uint32) (uint16, uint32, error) {
	var lastErr error
	for step := uint16(1); step <= 3; step++ {
		ext := internal + step
		if used[ext] {
			continue
		}
		lastErr = g.addSpecific(ctx, ext, proto, internal, client, lease)
		if lastErr == nil {
			return ext, lease, nil
		}
	}
	return 0, 0, cmp.Or(lastErr, errors.New("no free UPnP port"))
}

func (g *igdGateway) addSpecific(ctx context.Context, ext uint16, proto string, internal uint16, client string, lease uint32) error {
	callCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	return g.client.AddPortMappingCtx(callCtx, "", ext, proto, internal, client, true, "SoloDawn", lease)
}

func (g *igdGateway) deletePort(ctx context.Context, proto string, _ int, external uint16) {
	callCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	_ = g.client.DeletePortMappingCtx(callCtx, "", external, strings.ToUpper(proto))
}

func isPortRefused(err error) bool {
	var fault *soap.SOAPFaultError
	if !errors.As(err, &fault) {
		return false
	}
	code := fault.Detail.UPnPError.Errorcode
	return code == upnpConflict || code == upnpUnauthorized
}
