//go:build darwin

package node

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	socketfilterfwPath = "/usr/libexec/ApplicationFirewall/socketfilterfw"
	openBin            = "/usr/bin/open"
	lanProbe           = probeLocalNetwork
	lanTarget          = linkLocalTargets
	lanSend            = sendLinkLocalDiscard
	settingsURLs       = map[string]string{
		"local_network": "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension",
		"incoming":      "x-apple.systempreferences:com.apple.Network-Settings.extension",
		"folders":       "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_FilesAndFolders",
	}
)

type (
	darwinPermit struct {
		exe    string
		bundle string
	}
	stdTicker    struct{ *time.Ticker }
	darwinHelper struct {
		stdin io.WriteCloser
		done  chan struct{}
		once  sync.Once
	}
)

func newStdTicker(d time.Duration) permitTicker { return stdTicker{time.NewTicker(d)} }
func (t stdTicker) C() <-chan time.Time         { return t.Ticker.C }

func newPermitOps(exe string) *permitOps {
	bundle := bundleOf(exe)
	if bundle == "" {
		return nil
	}
	d := darwinPermit{exe: exe, bundle: bundle}
	return &permitOps{
		probeAll:     d.probeAll,
		openSettings: openDarwinSettings,
		startHelper:  d.startHelper,
		probeFolder:  d.probeFolder,
		now:          time.Now,
		newTicker:    newStdTicker,
	}
}

func (d darwinPermit) probeAll() proto.Permissions {
	return proto.Permissions{
		LocalNetwork: lanProbe(),
		Incoming:     d.probeIncoming(),
		FullDisk:     d.probeFullDisk(),
	}
}

func probeLocalNetwork() bool {
	return probeLocalNetworkWith(lanTarget, lanSend)
}

func probeLocalNetworkWith(listTargets func() ([]*net.UDPAddr, error), sendUDP func(*net.UDPAddr) error) bool {
	addrs, err := listTargets()
	if err != nil {
		return true
	}
	var successes, refusals int
	for _, addr := range addrs {
		s, r := countTargetProbe(addr, sendUDP)
		successes += s
		refusals += r
	}
	return successes >= 1 || refusals == 0
}

func countTargetProbe(addr *net.UDPAddr, sendUDP func(*net.UDPAddr) error) (int, int) {
	err := sendUDP(addr)
	if err == nil {
		return 1, 0
	}
	if errors.Is(err, syscall.EHOSTUNREACH) {
		return 0, 1
	}
	return 0, 0
}

func linkLocalTargets() ([]*net.UDPAddr, error) {
	return collectLinkLocalTargets(net.Interfaces, (*net.Interface).Addrs)
}

func collectLinkLocalTargets(
	listIfaces func() ([]net.Interface, error),
	listAddrs func(*net.Interface) ([]net.Addr, error),
) ([]*net.UDPAddr, error) {
	ifaces, err := listIfaces()
	if err != nil {
		return nil, err
	}
	var out []*net.UDPAddr
	for i := range ifaces {
		if addr := linkLocalTarget(&ifaces[i], listAddrs); addr != nil {
			out = append(out, addr)
		}
	}
	return out, nil
}

func linkLocalTarget(iface *net.Interface, listAddrs func(*net.Interface) ([]net.Addr, error)) *net.UDPAddr {
	const want = net.FlagUp | net.FlagBroadcast
	if iface.Flags&want != want {
		return nil
	}
	addrs, err := listAddrs(iface)
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ip := linkLocalV6Addr(a); ip != nil {
			return randomDiscardAddr(ip, iface.Name)
		}
	}
	return nil
}

func linkLocalV6Addr(a net.Addr) net.IP {
	ipnet, ok := a.(*net.IPNet)
	if !ok || ipnet.IP.To4() != nil || !ipnet.IP.IsLinkLocalUnicast() {
		return nil
	}
	return ipnet.IP.To16()
}

func randomDiscardAddr(prefix net.IP, zone string) *net.UDPAddr {
	dst := make(net.IP, net.IPv6len)
	copy(dst[:8], prefix[:8])
	_, _ = rand.Read(dst[8:])
	return &net.UDPAddr{IP: dst, Port: 9, Zone: zone}
}

func sendLinkLocalDiscard(raddr *net.UDPAddr) error {
	conn, err := net.DialUDP("udp6", nil, raddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte{0})
	return err
}

func (d darwinPermit) probeIncoming() bool {
	globalOut := cmdCombinedOutput(socketfilterfwPath, "--getglobalstate")
	exeOut := cmdCombinedOutput(socketfilterfwPath, "--getappblocked", d.exe)
	bundleOut := cmdCombinedOutput(socketfilterfwPath, "--getappblocked", d.bundle)
	return firewallAllowed(globalOut, exeOut) && firewallAllowed(globalOut, bundleOut)
}

func cmdCombinedOutput(bin string, args ...string) string {
	out, _ := exec.Command(bin, args...).CombinedOutput()
	return string(out)
}

func firewallAllowed(globalState, appBlocked string) bool {
	return !(strings.Contains(appBlocked, "is blocked") && !strings.Contains(globalState, "State = 0"))
}

func (d darwinPermit) probeFullDisk() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return exec.Command(d.exe, "probe", filepath.Join(home, "Library", "Safari")).Run() == nil
}

func (d darwinPermit) probeFolder(ctx context.Context, folder string) error {
	return probeCmdResult(ctx, folder, exec.CommandContext(ctx, d.exe, "probe", folder).Run())
}

func probeCmdResult(ctx context.Context, folder string, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 3 {
			return &host.Error{Code: "permission", Message: "macOS does not allow SoloDawn to use " + folder}
		}
		return nil
	}
	return err
}

func openDarwinSettings(target string) error {
	return exec.Command(openBin, settingsURLs[target]).Run()
}

func (d darwinPermit) startHelper(ctx context.Context, lang string) (permitHelper, error) {
	helperBin := filepath.Join(d.bundle, "Contents", "MacOS", "SoloDawnPermissions")
	return startHelperCmd(exec.CommandContext(ctx, helperBin, "--lang", lang))
}

func startHelperCmd(cmd *exec.Cmd) (permitHelper, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	w := log.Writer()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return &darwinHelper{stdin: stdin, done: done}, nil
}

func (h *darwinHelper) Grant() error {
	var err error
	h.once.Do(func() {
		_, writeErr := io.WriteString(h.stdin, "ok\n")
		closeErr := h.stdin.Close()
		err = errors.Join(writeErr, closeErr)
	})
	return err
}

func (h *darwinHelper) Done() <-chan struct{} { return h.done }
