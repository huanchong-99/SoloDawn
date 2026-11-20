//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The signers of the wire examples in internal/proto/testdata.
func exampleKey(name string) identity.Key {
	seed := sha256.Sum256([]byte("SoloDawn protocol example: " + name))
	return identity.Key(ed25519.NewKeyFromSeed(seed[:]))
}

func logEntry(signer, user identity.Key, ver int, op string, dev identity.Key) (proto.Envelope, error) {
	b, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: fmt.Sprint("entry-", ver, op), Timestamp: 1763600000000},
		DeviceChange:  proto.DeviceChange{User: user.ID(), Version: ver, Op: op, Device: dev.ID()},
	})
	return signer.Sign(string(b)), err
}

func deviceLog(user identity.Key, entries ...proto.Envelope) (*identity.Log, error) {
	return identity.Replay(user.ID(), entries)
}

func identitySteps(sc *godog.ScenarioContext) {
	browser, node, tablet := exampleKey("browser"), exampleKey("node"), exampleKey("tablet")
	now := time.UnixMilli(1763600000000)

	var dir string
	var first, second identity.Key
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if dir != "" {
			os.RemoveAll(dir)
		}
		return ctx, err
	})
	sc.Step(`^a node that started for the first time in an empty data directory$`, func() (err error) {
		if dir, err = os.MkdirTemp("", "solodawn-identity"); err != nil {
			return err
		}
		first, err = identity.LoadKey(dir)
		return err
	})
	sc.Step(`^the node starts again$`, func() (err error) {
		second, err = identity.LoadKey(dir)
		return err
	})
	sc.Step(`^it has the same device id and fingerprint as before$`, func() error {
		if len(first.ID()) != 43 || second.ID() != first.ID() || second.Fingerprint() != first.Fingerprint() {
			return fmt.Errorf("first start: %s (%s), second start: %s (%s)", first.ID(), first.Fingerprint(), second.ID(), second.Fingerprint())
		}
		return nil
	})
	sc.Step(`^its key file can be read by the user only$`, func() error {
		info, err := os.Stat(filepath.Join(dir, "device.key"))
		if err != nil {
			return err
		}
		// Windows has no owner-only mode bits; there the per-user data directory protects the file.
		if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
			return fmt.Errorf("device.key has mode %o", perm)
		}
		return nil
	})

	// One person's log, a signed payload on its way to a receiver, and the receiver's answer.
	var log *identity.Log
	var env proto.Envelope
	var receiver string
	var arrival time.Time
	var answer string
	read := func(open func(proto.Envelope, string, func(string) bool, time.Time) (proto.PayloadHeader, error)) {
		answer = "accepted"
		if _, err := open(env, receiver, log.Valid, arrival); err != nil {
			answer = err.Error()
		}
	}
	open := func() { read(identity.OpenCommand) }
	owned := func() (err error) {
		genesis, err := logEntry(browser, browser, 1, "add", browser)
		if err != nil {
			return err
		}
		addNode, err := logEntry(browser, browser, 2, "add", node)
		if err != nil {
			return err
		}
		log, err = deviceLog(browser, genesis, addNode)
		return err
	}
	change := func() { env.Payload = strings.Replace(env.Payload, "true", "false", 1) }
	revokeBrowser := func() error {
		revoke, err := logEntry(node, browser, 3, "revoke", browser)
		if err != nil {
			return err
		}
		_, err = log.Apply(revoke)
		return err
	}
	misdirect := func() { receiver = tablet.ID() }
	delay := func() { arrival = now.Add(25 * time.Hour) }
	sc.Step(`^a command that the owner's browser signed for the owner's node$`, func() error {
		env = browser.Sign(fmt.Sprintf(`{"t":"settings.set","n":"command-1","ts":%d,"to":"%s","offline":true}`, now.UnixMilli(), node.ID()))
		receiver, arrival = node.ID(), now
		return owned()
	})
	sc.Step(`^it arrives unchanged$`, func() { open() })
	sc.Step(`^one byte of its payload is changed on the way$`, func() { change(); open() })
	sc.Step(`^the browser is revoked before it arrives$`, func() error {
		defer open()
		return revokeBrowser()
	})
	sc.Step(`^it reaches another device$`, func() { misdirect(); open() })
	sc.Step(`^it arrives 25 hours later$`, func() { delay(); open() })
	sc.Step(`^it is changed, revoked, misdirected and late$`, func() error {
		change()
		misdirect()
		delay()
		defer open()
		return revokeBrowser()
	})
	sc.Step(`^it is revoked, misdirected and late$`, func() error {
		misdirect()
		delay()
		defer open()
		return revokeBrowser()
	})
	sc.Step(`^it is misdirected and late$`, func() { misdirect(); delay(); open() })
	sc.Step(`^the answer is "([^"]*)"$`, func(want string) error {
		if answer != want {
			return fmt.Errorf("the answer is %q", answer)
		}
		return nil
	})
	sc.Step(`^a payload that the owner's browser signed (\d+) minutes ago$`, func(minutes int) error {
		env = browser.Sign(fmt.Sprintf(`{"t":"post","n":"post-1","ts":%d,"g":"group-1","text":"hello"}`, now.Add(-time.Duration(minutes)*time.Minute).UnixMilli()))
		receiver, arrival = node.ID(), now
		return owned()
	})
	sc.Step(`^read as (signalling|a command) the answer is "([^"]*)"$`, func(kind, want string) error {
		if kind == "signalling" {
			read(identity.OpenSignal)
		} else {
			read(identity.OpenCommand)
		}
		if answer != want {
			return fmt.Errorf("the answer is %q", answer)
		}
		return nil
	})

	valid := func(l *identity.Log) []string {
		var names []string
		for name, key := range map[string]identity.Key{"browser": browser, "node": node, "tablet": tablet} {
			if l.Valid(key.ID()) {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		return names
	}
	var revoke proto.Envelope
	sc.Step(`^the device log of a person with a browser, a tablet and a node$`, func() error {
		if err := owned(); err != nil {
			return err
		}
		addTablet, err := logEntry(node, browser, 3, "add", tablet)
		if err != nil {
			return err
		}
		if added, err := log.Apply(addTablet); err != nil || !added {
			return fmt.Errorf("the tablet was not added: %v", err)
		}
		if got := valid(log); !slices.Equal(got, []string{"browser", "node", "tablet"}) {
			return fmt.Errorf("valid devices: %v", got)
		}
		return nil
	})
	sc.Step(`^the node revokes the tablet$`, func() (err error) {
		if revoke, err = logEntry(node, browser, 4, "revoke", tablet); err != nil {
			return err
		}
		if added, err := log.Apply(revoke); err != nil || !added {
			return fmt.Errorf("the revoke was not accepted: %v", err)
		}
		return nil
	})
	sc.Step(`^the valid devices are the browser and the node$`, func() error {
		if got := valid(log); !slices.Equal(got, []string{"browser", "node"}) {
			return fmt.Errorf("valid devices: %v", got)
		}
		return nil
	})
	refused := func(signer identity.Key, op string, dev identity.Key) error {
		entry, err := logEntry(signer, browser, 5, op, dev)
		if err != nil {
			return err
		}
		if added, err := log.Apply(entry); err == nil || added || len(log.Entries) != 4 {
			return fmt.Errorf("the entry was not refused: added %v, %d entries", added, len(log.Entries))
		}
		return nil
	}
	sc.Step(`^an entry signed by the tablet is refused$`, func() error { return refused(tablet, "revoke", node) })
	sc.Step(`^an entry that adds the tablet again is refused$`, func() error { return refused(browser, "add", tablet) })
	sc.Step(`^the same revoke arriving a second time changes nothing$`, func() error {
		if added, err := log.Apply(revoke); err != nil || added || len(log.Entries) != 4 {
			return fmt.Errorf("added %v, error %v, %d entries", added, err, len(log.Entries))
		}
		return nil
	})
	sc.Step(`^replaying the log from its first entry gives the same valid devices$`, func() error {
		replayed, err := identity.Replay(log.User, log.Entries)
		if err != nil {
			return err
		}
		if got := valid(replayed); !slices.Equal(got, []string{"browser", "node"}) {
			return fmt.Errorf("valid devices after the replay: %v", got)
		}
		return nil
	})

	var created, typed identity.Secret
	pairMessage := func(s identity.Secret, role string, k identity.Key, kind string) proto.Pair {
		return proto.Pair{Type: "pair", Key: k.ID(), Kind: kind, Name: kind, Role: role, MAC: s.MAC(role, k.ID())}
	}
	sc.Step(`^a browser that created a pairing secret$`, func() error {
		created = identity.NewSecret()
		if shown := created.String(); len(shown) != 19 || strings.Count(shown, "-") != 3 {
			return fmt.Errorf("the secret is shown as %q", shown)
		}
		return nil
	})
	sc.Step(`^a node is given the secret as a person typed it, in lower case$`, func() (err error) {
		typed, err = identity.ParseSecret(strings.ToLower(created.String()))
		return err
	})
	sc.Step(`^both sides meet in the same room$`, func() error {
		if len(typed.Room()) != 32 || typed.Room() != created.Room() {
			return fmt.Errorf("rooms %q and %q", created.Room(), typed.Room())
		}
		return nil
	})
	sc.Step(`^each side accepts the other's pair message$`, func() error {
		if !typed.Verify(pairMessage(created, "a", browser, "browser"), "a") || !created.Verify(pairMessage(typed, "b", node, "node"), "b") {
			return fmt.Errorf("a pair message of the other side was refused")
		}
		return nil
	})
	sc.Step(`^neither side accepts its own pair message sent back$`, func() error {
		if created.Verify(pairMessage(created, "a", browser, "browser"), "b") || typed.Verify(pairMessage(typed, "b", node, "node"), "a") {
			return fmt.Errorf("a side accepted its own pair message")
		}
		return nil
	})
	sc.Step(`^a pair message made with another secret is refused$`, func() error {
		other := identity.NewSecret()
		if other == created || created.Verify(pairMessage(other, "b", tablet, "node"), "b") {
			return fmt.Errorf("a pair message made with %s was accepted for %s", other, created)
		}
		return nil
	})

	adoption := func(signer identity.Key) (bool, error) {
		genesis, err := logEntry(browser, browser, 1, "add", browser)
		if err != nil {
			return false, err
		}
		addTablet, err := logEntry(browser, browser, 2, "add", tablet)
		if err != nil {
			return false, err
		}
		addNode, err := logEntry(signer, browser, 3, "add", node)
		if err != nil {
			return false, err
		}
		l, err := deviceLog(browser, genesis, addTablet, addNode)
		if err != nil {
			return false, err
		}
		return l.Adopts(node.ID(), browser.ID()), nil
	}
	sc.Step(`^a node that belongs to nobody and has paired with a browser$`, func() {})
	sc.Step(`^it adopts the person of a log in which that browser added it$`, func() error {
		if adopted, err := adoption(browser); err != nil || !adopted {
			return fmt.Errorf("adopted %v, %v", adopted, err)
		}
		return nil
	})
	sc.Step(`^it does not adopt the person of a log in which another device added it$`, func() error {
		if adopted, err := adoption(tablet); err != nil || adopted {
			return fmt.Errorf("adopted %v, %v", adopted, err)
		}
		return nil
	})
}
