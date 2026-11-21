package spike

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

// Synthetic 32-byte key encoded like a device id.
const quotaID = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func allocate(t *testing.T, ice webrtc.ICEServer) (net.PacketConn, error) {
	socket := must(tls.Dial("tcp", turnName+":443", &tls.Config{ServerName: turnName}))
	t.Cleanup(func() { socket.Close() })
	client := must(turn.NewClient(&turn.ClientConfig{TURNServerAddr: turnName + ":443",
		Conn: turn.NewSTUNConn(socket), Username: ice.Username, Password: ice.Credential.(string)}))
	t.Cleanup(client.Close)
	check(client.Listen())
	c, err := client.Allocate()
	if err == nil {
		t.Cleanup(func() { c.Close() })
	}
	return c, err
}

func transfer(t *testing.T, from, to net.PacketConn, payload string, blocked bool) {
	t.Helper()
	if n := must(from.WriteTo([]byte(payload), to.LocalAddr())); n != len(payload) {
		t.Fatal("short write", n)
	}
	check(to.SetReadDeadline(time.Now().Add(500 * time.Millisecond)))
	b := make([]byte, 100)
	n, src, err := to.ReadFrom(b)
	if blocked {
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("expected deadline, got %q %v", b[:n], err)
		}
		return
	}
	if err != nil || string(b[:n]) != payload || src.String() != from.LocalAddr().String() {
		t.Fatalf("wanted %q from %v; got %q from %v (%v)", payload, from.LocalAddr(), b[:n], src, err)
	}
}

func probeQuota(t *testing.T, secret string, g *generator) {
	peer := must(net.ListenPacket("udp4", labIP+":0"))
	defer peer.Close()
	old := issue(secret, quotaID, 3*time.Second)
	exp, id, ok := strings.Cut(old.Username, ":")
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(old.Username))
	if !ok || id != quotaID || base64.StdEncoding.EncodeToString(mac.Sum(nil)) != old.Credential {
		t.Fatal("credential format")
	}
	first := must(allocate(t, old))
	transfer(t, first, peer, "old-send", false)
	transfer(t, peer, first, "old-back", false)
	t.Log("original allocation transferred 8 + 8 bytes; MAC matched")
	// Pion accepts the expiry second itself. Test the first subsequent second.
	time.Sleep(time.Until(time.Unix(must(strconv.ParseInt(exp, 10, 64))+1, 0)))
	if _, err := allocate(t, old); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expired Allocate = %v", err)
	} else {
		t.Log("expired Allocate:", err)
	}
	transfer(t, first, peer, "old-live", false)
	transfer(t, peer, first, "old-stay", false)
	t.Log("existing allocation after credential expiry transferred 8 + 8 bytes")
	fresh := issue(secret, quotaID, time.Hour)
	if fresh.Username == old.Username || fresh.Credential == old.Credential {
		t.Fatal("did not renew")
	}
	second := must(allocate(t, fresh))
	if first.LocalAddr().String() == second.LocalAddr().String() {
		t.Fatal("allocation reused")
	}
	transfer(t, second, peer, "new-send", false)
	transfer(t, peer, second, "new-back", false)
	t.Log("renewed credential, second allocation transferred 8 + 8 bytes")
	for _, c := range []net.PacketConn{first, second} {
		transfer(t, c, peer, "over-cap", true)
		transfer(t, peer, c, "cap-full", true)
	}
	control := must(allocate(t, issue(secret, "control", time.Hour)))
	transfer(t, control, peer, "separate", false)
	transfer(t, peer, control, "separate", false)
	m := g.accounts[quotaID]
	m.Lock()
	defer m.Unlock()
	if m.used != quotaLimit || m.droppedIn != 2 || m.droppedOut != 2 {
		t.Fatalf("used=%d drops=%d/%d", m.used, m.droppedIn, m.droppedOut)
	}
	t.Logf("used=%d/48; post-cap drops peer-to-client=%d client-to-peer=%d; control echoed", m.used, m.droppedIn, m.droppedOut)
}

// Two devices have independent 48-byte quotas in the isolated fixture.
// Charge both at the peer socket, outside the TURN framing.
type meter struct {
	sync.Mutex
	used, droppedIn, droppedOut int
}
type generator struct {
	*turn.RelayAddressGeneratorStatic
	accounts map[string]*meter
}

func (g *generator) AllocatePacketConn(c turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := g.RelayAddressGeneratorStatic.AllocatePacketConn(c)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("allocation device=%s relay=%s\n", c.UserID, addr)
	m := g.accounts[c.UserID]
	if m == nil {
		return conn, addr, nil
	}
	return &capped{conn, m}, addr, nil
}

type capped struct {
	net.PacketConn
	m *meter
}

func (c capped) WriteTo(p []byte, dst net.Addr) (int, error) {
	c.m.Lock()
	defer c.m.Unlock()
	if len(p) > quotaLimit-c.m.used {
		c.m.droppedOut++
		return len(p), nil
	}
	n, err := c.PacketConn.WriteTo(p, dst)
	c.m.used += n
	return n, err
}
func (c capped) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, src, err := c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, src, err
		}
		c.m.Lock()
		if n <= quotaLimit-c.m.used {
			c.m.used += n
			c.m.Unlock()
			return n, src, nil
		}
		c.m.droppedIn++
		c.m.Unlock()
	}
}
