//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["m8"] = m8Steps
}

type m8InstallWorld struct {
	f           fixtures
	cleanup     func(func())
	ctx         context.Context
	home        string
	dlDir       string
	asset       string
	image       []byte
	binPath     string
	sockPath    string
	env         []string
	rvURL       string
	browser     *peerBrowser
	nodeID      string
	fingerprint string
}

type m8CLIResult struct {
	stdout string
	stderr string
	err    error
}

func m8Steps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	iw := &m8InstallWorld{f: f, cleanup: cleanup}

	sc.Step(`^a clean user home with release "([^"]+)" and a local rendezvous$`, iw.setupCleanHome)
	sc.Step(`^the install script rejects a mismatching SHA256SUMS and installs "([^"]+)" once SHA256SUMS matches$`, iw.verifyAndInstall)
	sc.Step(`^a fresh browser pairs with the installed node via "solodawn pair" as "([^"]+)"$`, iw.pairInstalledNode)
	sc.Step(`^the current user has a login service registration and status matches the browser fingerprint$`, iw.checkServiceAndStatus)
	sc.Step(`^the installed node stops and starts again as after a reboot$`, iw.restartInstalledNode)
	sc.Step(`^the restarted node keeps its fingerprint, owner enrolment, and connected status$`, iw.checkPersistedNode)

	sc.Step(`^a fresh server running "solodawn rendezvous" with TLS, STUN, and TURN in one command$`, func(ctx context.Context) error {
		return startOneCommandRV(ctx, f)
	})
}

func (w *m8InstallWorld) setupCleanHome(tag string) error {
	if err := w.initDirs(tag); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := checkLoginRegistration(w.home, false); err != nil {
		return err
	}
	registerBothCleanups(w.f.t, w.cleanup, w.removeServiceIfPresent)
	registerBothCleanups(w.f.t, w.cleanup, func() { _ = m8StopExeNodes(w.binPath, w.sockPath) })
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	w.rvURL = rvURL
	return err
}

func (w *m8InstallWorld) initDirs(tag string) error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sd8i")
	if err != nil {
		return err
	}
	w.home = filepath.Join(root, "home")
	w.dlDir = filepath.Join(root, "dl")
	for _, sub := range []string{w.home, w.dlDir} {
		if err := os.MkdirAll(sub, 0o700); err != nil {
			return err
		}
	}
	image, err := stampedRelease(w.f, tag)
	if err != nil {
		return err
	}
	w.image, w.asset = image, m8AssetName(tag)
	if err := os.WriteFile(filepath.Join(w.dlDir, w.asset), image, 0o644); err != nil {
		return err
	}
	w.binPath, w.sockPath, w.env = m8ProfileLayout(w.home, w.f.fakeCLIs)
	return nil
}

func (w *m8InstallWorld) removeServiceIfPresent() {
	ok, err := loginRegistration(w.home)
	if err != nil || !ok {
		return
	}
	if runtime.GOOS == "windows" {
		_ = node.RemoveService()
		return
	}
	cmd := exec.Command(w.f.bin, "service", "remove")
	cmd.Env = w.env
	_ = cmd.Run()
}

func (w *m8InstallWorld) verifyAndInstall(tag string) error {
	goodSum := fmt.Sprintf("%x", sha256.Sum256(w.image))
	if err := w.writeSumsAndRunInstall(m8FlipHex(goodSum)); err == nil {
		return errors.New("install script succeeded with mismatching SHA256SUMS")
	}
	if _, err := os.Stat(w.binPath); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("binary installed despite checksum mismatch: %v", err)
	}
	if err := w.writeSumsAndRunInstall(goodSum); err != nil {
		return fmt.Errorf("install with matching SHA256SUMS: %w", err)
	}
	out, err := runNodeCLI(w.binPath, w.env, "version")
	if err != nil || out != tag+"\n" {
		return fmt.Errorf("solodawn version: out=%q err=%v", out, err)
	}
	return nil
}

func m8FlipHex(sum string) string {
	if strings.HasPrefix(sum, "0") {
		return "f" + sum[1:]
	}
	return "0" + sum[1:]
}

func (w *m8InstallWorld) writeSumsAndRunInstall(hexDigest string) error {
	sums := fmt.Sprintf("%s  %s\n", hexDigest, w.asset)
	if err := os.WriteFile(filepath.Join(w.dlDir, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		return err
	}
	cmd, err := m8InstallCommand(w.asset)
	if err != nil {
		return err
	}
	cmd.Dir, cmd.Env = w.dlDir, w.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

func (w *m8InstallWorld) pairInstalledNode(label string) error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey("m8-install-browser"), w.rvURL, nil)
	if err != nil {
		return err
	}
	w.browser = b
	secret := identity.NewSecret()
	done := w.startPairCLI(secret.String(), label)
	roomMsg, err := b.exchangeRoom(w.ctx, secret, "a", "b")
	if err != nil {
		return err
	}
	if err := w.completeBrowserAdopt(b, roomMsg, label); err != nil {
		return err
	}
	return w.awaitPairCLI(done, label)
}

func (w *m8InstallWorld) startPairCLI(secret, label string) <-chan m8CLIResult {
	pairCtx, cancel := context.WithCancel(w.ctx)
	// Single reader: awaitPairCLI.
	done := make(chan m8CLIResult, 1)
	var settled sync.WaitGroup
	settled.Add(1)
	go func() {
		defer settled.Done()
		out, stderr, err := m8ExecCLIContext(pairCtx, w.binPath, w.env, "pair", secret, "--name", label, "--rendezvous", w.rvURL)
		done <- m8CLIResult{stdout: out, stderr: stderr, err: err}
	}()
	registerBothCleanups(w.f.t, w.cleanup, func() {
		cancel()
		settled.Wait()
	})
	return done
}

func (w *m8InstallWorld) completeBrowserAdopt(b *peerBrowser, roomMsg proto.Pair, label string) error {
	fp, err := deviceFingerprint(roomMsg.Key)
	if err != nil {
		return err
	}
	w.nodeID, w.fingerprint = roomMsg.Key, fp
	bc, err := b.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if _, err := requestBrowserHello(w.ctx, bc.conn); err != nil {
		return err
	}
	ts := time.Now().UnixMilli()
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(b.key, b.key.ID(), 1, "browser", "Owner Browser", b.key.ID(), ts),
		nodetest.SignDeviceAdd(b.key, b.key.ID(), 2, "node", label, w.nodeID, ts),
	}
	if err := bc.conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: entries}); err != nil {
		return err
	}
	return checkOwnerWelcome(w.ctx, bc.conn, w.nodeID)
}

func (w *m8InstallWorld) awaitPairCLI(done <-chan m8CLIResult, label string) error {
	select {
	case res := <-done:
		return w.verifyPairCLIResult(res, label)
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *m8InstallWorld) verifyPairCLIResult(res m8CLIResult, label string) error {
	if res.err != nil || res.stderr != "" {
		return fmt.Errorf("solodawn pair: err=%v stderr=%q stdout=%q", res.err, res.stderr, res.stdout)
	}
	if !strings.Contains(res.stdout, label) || !strings.Contains(res.stdout, w.fingerprint) {
		return fmt.Errorf("solodawn pair output %q missing %q or %q", res.stdout, label, w.fingerprint)
	}
	return nil
}

func (w *m8InstallWorld) checkServiceAndStatus() error {
	if err := checkLoginRegistration(w.home, true); err != nil {
		return err
	}
	return w.assertStatusConnected()
}

func (w *m8InstallWorld) assertStatusConnected() error {
	out, err := runNodeCLI(w.binPath, w.env, "status")
	if err != nil {
		return err
	}
	if !w.statusMatches(out) {
		return fmt.Errorf("unexpected status output: %q", out)
	}
	return nil
}

func (w *m8InstallWorld) statusMatches(out string) bool {
	hasVer := strings.Contains(out, "version v1.0.0\n")
	hasFP := strings.Contains(out, "fingerprint "+w.fingerprint+"\n")
	hasConn := strings.Contains(out, "connection "+w.browser.key.ID()+" ")
	return hasVer && hasFP && hasConn
}

func (w *m8InstallWorld) restartInstalledNode() error {
	if err := m8StopExeNodes(w.binPath, w.sockPath); err != nil {
		return err
	}
	if _, err := runNodeCLI(w.binPath, w.env, "status"); err == nil {
		return errors.New("status succeeded while node was stopped")
	}
	return w.startForegroundNode()
}

func (w *m8InstallWorld) startForegroundNode() error {
	var logBuf syncBuffer
	cmd := exec.Command(w.binPath, "node")
	cmd.Env = w.env
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	if _, err := startManagedCmd(w.f.t, w.cleanup, cmd); err != nil {
		return err
	}
	if err := waitAcceptanceSocket(w.sockPath, 10*time.Second); err != nil {
		return fmt.Errorf("restarted node socket: %w (%s)", err, logBuf.String())
	}
	return nil
}

func (w *m8InstallWorld) checkPersistedNode() error {
	if err := w.browser.waitOnline(w.ctx, w.nodeID); err != nil {
		return err
	}
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if err := w.verifyReconnectedNode(bc.conn); err != nil {
		return err
	}
	return w.assertStatusConnected()
}

func (w *m8InstallWorld) verifyReconnectedNode(conn *peer.Connection) error {
	if err := checkOwnerWelcome(w.ctx, conn, w.nodeID); err != nil {
		return err
	}
	info, err := requestHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	if info.Device != w.nodeID || info.Name != "workstation" || info.Version != "v1.0.0" {
		return fmt.Errorf("restarted host.info = %+v", info)
	}
	return nil
}

func startOneCommandRV(ctx context.Context, f fixtures) error {
	rv := ctx.Value(rvKey{}).(*rvWorld)
	rvDir, err := shortTempRoot(f.t, rv.addCleanup, "sd8rv")
	if err != nil {
		return err
	}
	roots, err := localCert(rvDir)
	if err != nil {
		return err
	}
	installOneCommandRootCA(rv.addCleanup, roots)
	wsURL, err := launchOneCommandRV(f, rv.addCleanup, rvDir)
	if err != nil {
		return err
	}
	rv.ws = wsURL
	return verifyOneCommandRVICE(wsURL)
}

func installOneCommandRootCA(cleanup func(func()), roots *x509.CertPool) {
	prev := http.DefaultClient.Transport
	ts := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"},
	}
	http.DefaultClient.Transport = ts
	cleanup(func() {
		ts.CloseIdleConnections()
		http.DefaultClient.Transport = prev
	})
}

func launchOneCommandRV(f fixtures, cleanup func(func()), rvDir string) (string, error) {
	port, err := freePort()
	if err != nil {
		return "", err
	}
	addr := "127.0.0.1:" + port
	rvHome := filepath.Join(rvDir, "home")
	if err := os.MkdirAll(rvHome, 0o700); err != nil {
		return "", err
	}
	cmd := exec.Command(
		f.bin, "rendezvous",
		"--listen", addr,
		"--domain", "localhost",
		"--turn-domain", "turn.localhost",
		"--cert", filepath.Join(rvDir, "cert"),
		"--key", filepath.Join(rvDir, "key"),
		"--udp-port", "0",
	)
	cmd.Env = m8CleanEnv(map[string]string{"HOME": rvHome, "XDG_CACHE_HOME": rvHome, "APPDATA": rvHome, "LOCALAPPDATA": rvHome})
	if err := killManagedCmd(f.t, cleanup, cmd, nil); err != nil {
		return "", err
	}
	return "wss://" + addr + "/rv", rvListening(addr)
}

func verifyOneCommandRVICE(wsURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path, err := peer.Dial(ctx, wsURL, exampleKey("m8-rv-probe"))
	if err != nil {
		return err
	}
	defer path.Close()
	return m8CheckReadyICE(path.Route().ICE)
}

func m8CheckReadyICE(servers []proto.ICEServer) error {
	raw, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	s := string(raw)
	for _, want := range []string{"stun:localhost:", "turn:localhost:", "turns:turn.localhost:"} {
		if !strings.Contains(s, want) {
			return fmt.Errorf("ready ICE %s missing %q", s, want)
		}
	}
	return nil
}

func m8AssetName(tag string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return "solodawn_" + tag + "_" + runtime.GOOS + "_" + runtime.GOARCH + ext
}

func m8ProfileLayout(home, fakeCLIs string) (binPath, sockPath string, env []string) {
	dataHome := filepath.Join(home, "data")
	appData := filepath.Join(home, "appdata")
	binDir, binName, sockDir := m8PlatformDirs(home, dataHome, appData)
	pathVal := strings.Join([]string{binDir, fakeCLIs, os.Getenv("PATH")}, string(os.PathListSeparator))
	env = m8CleanEnv(map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CONFIG_HOME": home,
		"XDG_DATA_HOME":   dataHome,
		"APPDATA":         appData,
		"LOCALAPPDATA":    appData,
		"PATH":            pathVal,
	})
	return filepath.Join(binDir, binName), filepath.Join(sockDir, "sock"), env
}

func m8PlatformDirs(home, dataHome, appData string) (binDir, binName, sockDir string) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(appData, `Programs\SoloDawn`), "solodawn.exe", filepath.Join(appData, "solodawn")
	case "darwin":
		return filepath.Join(home, ".local", "bin"), "solodawn", filepath.Join(home, "Library", "Application Support", "solodawn")
	default:
		return filepath.Join(home, ".local", "bin"), "solodawn", filepath.Join(dataHome, "solodawn")
	}
}

func m8CleanEnv(overrides map[string]string) []string {
	var out []string
	for _, entry := range os.Environ() {
		k, _, _ := strings.Cut(entry, "=")
		if !m8HasKeyFold(overrides, k) {
			out = append(out, entry)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(overrides)) {
		out = append(out, k+"="+overrides[k])
	}
	return out
}

func m8HasKeyFold(m map[string]string, target string) bool {
	for k := range m {
		if strings.EqualFold(k, target) {
			return true
		}
	}
	return false
}

func m8InstallCommand(asset string) (*exec.Cmd, error) {
	raw, err := os.ReadFile(filepath.Join("..", "install.md"))
	if err != nil {
		return nil, err
	}
	doc := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if runtime.GOOS == "windows" {
		script, err := m8LoadFencedInstall(doc, "powershell", "REPLACE_WITH_BINARY_FILENAME.exe", asset)
		if err != nil {
			return nil, err
		}
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script), nil
	}
	script, err := m8LoadFencedInstall(doc, "sh", "REPLACE_WITH_BINARY_FILENAME", asset)
	if err != nil {
		return nil, err
	}
	return exec.Command("sh", "-c", script), nil
}

func m8LoadFencedInstall(doc, lang, placeholder, asset string) (string, error) {
	fence := "```" + lang + "\n"
	_, rest, ok := strings.Cut(doc, fence)
	if !ok {
		return "", fmt.Errorf("install.md missing %s block", lang)
	}
	block, _, ok := strings.Cut(rest, "\n```")
	if !ok || !strings.Contains(block, placeholder) {
		return "", fmt.Errorf("install.md %s block missing %s", lang, placeholder)
	}
	return strings.Replace(block, placeholder, asset, 1) + "\n", nil
}

func m8ExecCLIContext(ctx context.Context, bin string, env []string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func m8StopExeNodes(exe, sock string) error {
	if runtime.GOOS == "windows" {
		return m8StopExeWindows(exe, sock)
	}
	return m8StopExeUnix(exe, sock)
}

func m8StopExeWindows(exe, sock string) error {
	script := `Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $env:SD_STOP_EXE } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "SD_STOP_EXE="+exe)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stop %s: %w (%s)", exe, err, bytes.TrimSpace(out))
	}
	return m8WaitSocketGone(sock, 5*time.Second)
}

func m8StopExeUnix(exe, sock string) error {
	m8SignalUnixPIDs(m8FindUnixPIDs(exe), os.Interrupt)
	if m8WaitUnixStopped(exe, sock, 3*time.Second) {
		return nil
	}
	m8SignalUnixPIDs(m8FindUnixPIDs(exe), os.Kill)
	if m8WaitUnixStopped(exe, sock, 2*time.Second) {
		return nil
	}
	return fmt.Errorf("processes for %s did not stop", exe)
}

func m8SignalUnixPIDs(pids []int, sig os.Signal) {
	for _, pid := range pids {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(sig)
		}
	}
}

func m8WaitUnixStopped(exe, sock string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if len(m8FindUnixPIDs(exe)) == 0 && !m8CanDialUnix(sock) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func m8WaitSocketGone(sock string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if !m8CanDialUnix(sock) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("socket %s stayed reachable", sock)
}

func m8CanDialUnix(sock string) bool {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func m8FindUnixPIDs(exe string) []int {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,stat=,command=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		if pid, ok := m8MatchLivePID(line, exe); ok {
			pids = append(pids, pid)
		}
	}
	return pids
}

func m8MatchLivePID(line, exe string) (int, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || strings.HasPrefix(fields[1], "Z") {
		return 0, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || fields[2] != exe {
		return 0, false
	}
	return pid, true
}
