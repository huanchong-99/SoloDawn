package node

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestDarwinNewPermitOpsAndIncomingMatrix(t *testing.T) {
	if newPermitOps("/usr/local/bin/solodawn") != nil {
		t.Fatal("non-bundle exe should yield nil permitOps")
	}
	if ops := newPermitOps("/Applications/SoloDawn.app/Contents/MacOS/solodawn"); ops == nil {
		t.Fatal("bundle exe should yield non-nil permitOps")
	}

	// All 4 socketfilterfw combinations.
	for _, tc := range []struct {
		global, app string
		want        bool
	}{
		{"Firewall is enabled. (State = 1)", "Incoming connection to /x.app is blocked.", false},
		{"Firewall is enabled. (State = 1)", "Incoming connection to /x.app is permitted.", true},
		{"Firewall is disabled. (State = 0)", "Incoming connection to /x.app is blocked.", true},
		{"Firewall is disabled. (State = 0)", "Incoming connection to /x.app is permitted.", true},
	} {
		if got := firewallAllowed(tc.global, tc.app); got != tc.want {
			t.Errorf("firewallAllowed(%q, %q) = %v; want %v", tc.global, tc.app, got, tc.want)
		}
	}

	// P0 8.9: probeIncoming checks both exe and bundle paths.
	savedFW := socketfilterfwPath
	t.Cleanup(func() { socketfilterfwPath = savedFW })
	fwDir := t.TempDir()
	fwScript := filepath.Join(fwDir, "socketfilterfw")
	fwGlobal := filepath.Join(fwDir, "global")
	fwExe := filepath.Join(fwDir, "exe")
	fwBundle := filepath.Join(fwDir, "bundle")
	if err := os.WriteFile(fwScript, []byte("#!/bin/sh\nif [ \"$1\" = \"--getglobalstate\" ]; then cat "+fwGlobal+"; exit 0; fi\ncase \"$2\" in\n  */MacOS/*) cat "+fwExe+" ;;\n  *) cat "+fwBundle+" ;;\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	socketfilterfwPath = fwScript
	d := darwinPermit{exe: "/Applications/SoloDawn.app/Contents/MacOS/solodawn", bundle: "/Applications/SoloDawn.app"}
	for _, tc := range []struct {
		name, global, exeOut, bundleOut string
		want                            bool
	}{
		{"exe blocked only", "Firewall is enabled. (State = 1)\n", "Incoming connection to exe is blocked.\n", "Incoming connection to app is permitted.\n", false},
		{"bundle blocked only", "Firewall is enabled. (State = 1)\n", "Incoming connection to exe is permitted.\n", "Incoming connection to app is blocked.\n", false},
		{"both permitted", "Firewall is enabled. (State = 1)\n", "Incoming connection to exe is permitted.\n", "Incoming connection to app is permitted.\n", true},
		{"firewall disabled", "Firewall is disabled. (State = 0)\n", "Incoming connection to exe is blocked.\n", "Incoming connection to app is blocked.\n", true},
	} {
		if err := os.WriteFile(fwGlobal, []byte(tc.global), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fwExe, []byte(tc.exeOut), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fwBundle, []byte(tc.bundleOut), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := d.probeIncoming(); got != tc.want {
			t.Errorf("%s: probeIncoming() = %v; want %v", tc.name, got, tc.want)
		}
	}
	socketfilterfwPath = savedFW
}

func TestDarwinLocalNetworkProbe(t *testing.T) {
	savedTarget, savedSend := lanTarget, lanSend
	t.Cleanup(func() {
		lanTarget = savedTarget
		lanSend = savedSend
	})

	_, v4Net, _ := net.ParseCIDR("192.168.1.5/24")
	_, globalV6, _ := net.ParseCIDR("2001:db8::1/64")
	llIP, llNet, _ := net.ParseCIDR("fe80::1/64")
	llNet.IP = llIP

	ifUp := net.Flags(net.FlagUp | net.FlagBroadcast)
	ifDown := net.Flags(net.FlagBroadcast)
	ifNoBcast := net.Flags(net.FlagUp)

	// 0. Passive interface listing works without sending any LAN datagrams.
	if _, err := linkLocalTargets(); err != nil {
		t.Fatalf("linkLocalTargets = %v", err)
	}

	// 1. Target collection filters non-broadcast/down/non-fe80/error interfaces and keeps fe80:: zones.
	if _, err := collectLinkLocalTargets(
		func() ([]net.Interface, error) { return nil, errors.New("ifaces err") },
		nil,
	); err == nil {
		t.Fatal("expected listIfaces error")
	}
	targets, err := collectLinkLocalTargets(
		func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "down0", Flags: ifDown},
				{Name: "lo0", Flags: ifNoBcast},
				{Name: "err0", Flags: ifUp},
				{Name: "v4", Flags: ifUp},
				{Name: "en0", Flags: ifUp},
				{Name: "llw0", Flags: ifUp},
			}, nil
		},
		func(i *net.Interface) ([]net.Addr, error) {
			switch i.Name {
			case "err0":
				return nil, errors.New("addrs err")
			case "v4":
				return []net.Addr{&net.TCPAddr{IP: net.IPv4(1, 2, 3, 4)}, v4Net, globalV6}, nil
			default:
				return []net.Addr{llNet}, nil
			}
		},
	)
	if err != nil || len(targets) != 2 || targets[0].Zone != "en0" || targets[1].Zone != "llw0" {
		t.Fatalf("collectLinkLocalTargets = %v (%v)", targets, err)
	}

	// 2. probeLocalNetwork decision matrix via faked targets + faked sendUDP (no real LAN packets).
	lanTarget = func() ([]*net.UDPAddr, error) { return nil, errors.New("ifaces err") }
	if !probeLocalNetwork() {
		t.Fatal("target listing err should return true")
	}
	lanTarget = func() ([]*net.UDPAddr, error) { return targets, nil }
	lanSend = func(addr *net.UDPAddr) error {
		if addr.Zone == "llw0" {
			return &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}
		}
		return &net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EHOSTUNREACH)}
	}
	if probeLocalNetwork() {
		t.Fatal("all EHOSTUNREACH should return false")
	}
	lanSend = func(addr *net.UDPAddr) error {
		if addr.Zone == "llw0" {
			return syscall.EHOSTUNREACH
		}
		return nil
	}
	if !probeLocalNetwork() {
		t.Fatal("1 success + 1 EHOSTUNREACH should return true")
	}
	lanSend = func(*net.UDPAddr) error { return syscall.ENETDOWN }
	if !probeLocalNetwork() {
		t.Fatal("non-EHOSTUNREACH error should return true")
	}

	// 3. sendLinkLocalDiscard: loopback listener succeeds; bad zone fails DialUDP locally.
	pc, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	if err := sendLinkLocalDiscard(pc.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("loopback sendLinkLocalDiscard = %v", err)
	}
	if err := sendLinkLocalDiscard(&net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 9, Zone: "badzone999"}); err == nil {
		t.Fatal("expected DialUDP error for nonexistent zone")
	}
}

func TestDarwinProbesHelperAndOpen(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "SoloDawn.app")
	macOS := filepath.Join(bundle, "Contents", "MacOS")
	if err := os.MkdirAll(macOS, 0o700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(macOS, "solodawn")
	codeFile := filepath.Join(root, "exitcode")
	mustWriteScript := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mustWriteScript(exe, "#!/bin/sh\nif [ \"$1\" = \"probe\" ]; then\n  c=$(cat "+codeFile+" 2>/dev/null || echo 0)\n  if [ \"$c\" = \"sleep\" ]; then sleep 5; exit 0; fi\n  exit \"$c\"\nfi\nexit 1\n")

	d := darwinPermit{exe: exe, bundle: bundle}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// exit 0 -> allowed + full_disk true
	if err := os.WriteFile(codeFile, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.probeFolder(t.Context(), "/tmp/f"); err != nil {
		t.Fatalf("exit 0 probeFolder = %v", err)
	}
	if !d.probeFullDisk() {
		t.Fatal("exit 0 probeFullDisk = false")
	}

	// exit 3 -> permission error + full_disk false
	if err := os.WriteFile(codeFile, []byte("3"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := d.probeFolder(t.Context(), "/Users/me/Documents")
	var wireErr *host.Error
	if !errors.As(err, &wireErr) || wireErr.Code != "permission" || wireErr.Message != "macOS does not allow SoloDawn to use /Users/me/Documents" {
		t.Fatalf("exit 3 probeFolder = %v", err)
	}
	if d.probeFullDisk() {
		t.Fatal("exit 3 probeFullDisk = true")
	}

	// N2-3: exit 1 and 2 allow probeFolder (not exit 3) but report full_disk false (only exit 0 is true).
	for _, code := range []string{"1", "2"} {
		if err := os.WriteFile(codeFile, []byte(code), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := d.probeFolder(t.Context(), "/tmp/f"); err != nil {
			t.Fatalf("exit %s probeFolder = %v", code, err)
		}
		if d.probeFullDisk() {
			t.Fatalf("exit %s probeFullDisk = true; want false", code)
		}
	}

	// timeout -> context error
	if err := os.WriteFile(codeFile, []byte("sleep"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := d.probeFolder(waitCtx, "/tmp/f"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sleep probeFolder = %v", err)
	}

	// missing exe -> exec error + full_disk false
	missing := darwinPermit{exe: filepath.Join(root, "missing"), bundle: bundle}
	if err := missing.probeFolder(t.Context(), "/tmp/f"); err == nil {
		t.Fatal("missing exe should fail")
	}

	// missing HOME -> probeFullDisk false
	t.Setenv("HOME", "")
	if d.probeFullDisk() {
		t.Fatal("missing home probeFullDisk = true")
	}
	t.Setenv("HOME", home)

	// openDarwinSettings with fake /usr/bin/open replacement.
	savedOpen := openBin
	t.Cleanup(func() { openBin = savedOpen })
	openLog := filepath.Join(root, "open.log")
	openScript := filepath.Join(root, "open")
	mustWriteScript(openScript, "#!/bin/sh\nprintf '%s\\n' \"$1\" >> "+openLog+"\n")
	openBin = openScript
	for _, target := range []string{"local_network", "incoming", "folders"} {
		if err := openDarwinSettings(target); err != nil {
			t.Fatalf("openDarwinSettings(%q) = %v", target, err)
		}
	}
	gotURLs, err := os.ReadFile(openLog)
	if err != nil {
		t.Fatal(err)
	}
	wantURLs := strings.Join([]string{
		"x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension",
		"x-apple.systempreferences:com.apple.Network-Settings.extension",
		"x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_FilesAndFolders",
	}, "\n") + "\n"
	if string(gotURLs) != wantURLs {
		t.Fatalf("opened URLs = %q; want %q", gotURLs, wantURLs)
	}

	// Helper lifecycle: startHelperCmd errors, startHelper with --lang, Grant() closes pipe.
	if _, err := d.startHelper(t.Context(), "en"); err == nil {
		t.Fatal("missing SoloDawnPermissions should fail")
	}
	if _, err := startHelperCmd(&exec.Cmd{Stdin: strings.NewReader("")}); err == nil {
		t.Fatal("preset Stdin should fail StdinPipe")
	}
	helperLog := filepath.Join(root, "helper.log")
	helperBin := filepath.Join(macOS, "SoloDawnPermissions")
	mustWriteScript(helperBin, "#!/bin/sh\necho \"$1 $2\" >> "+helperLog+"\nIFS= read -r line\necho \"$line\" >> "+helperLog+"\n")
	h, err := d.startHelper(t.Context(), "zh-CN")
	if err != nil {
		t.Fatalf("startHelper = %v", err)
	}
	if err := h.Grant(); err != nil {
		t.Fatalf("Grant = %v", err)
	}
	if err := h.Grant(); err != nil {
		t.Fatalf("second Grant = %v", err)
	}
	<-h.Done()
	gotHelper, err := os.ReadFile(helperLog)
	if err != nil || string(gotHelper) != "--lang zh-CN\nok\n" {
		t.Fatalf("helper log = %q (%v)", gotHelper, err)
	}

	// N2-1: d.probeAll with faked lanProbe and faked socketfilterfwPath (zero real LAN packets or macOS queries).
	savedLAN, savedFW := lanProbe, socketfilterfwPath
	t.Cleanup(func() {
		lanProbe = savedLAN
		socketfilterfwPath = savedFW
	})
	fwScript := filepath.Join(root, "socketfilterfw")
	mustWriteScript(fwScript, "#!/bin/sh\necho 'Firewall is disabled. (State = 0)'\n")
	lanProbe = func() bool { return true }
	socketfilterfwPath = fwScript
	if err := os.WriteFile(codeFile, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	all := d.probeAll()
	if all != (proto.Permissions{LocalNetwork: true, Incoming: true, FullDisk: true}) {
		t.Fatalf("probeAll = %+v", all)
	}
	tk := newStdTicker(time.Millisecond)
	<-tk.C()
	tk.Stop()
}
