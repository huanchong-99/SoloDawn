package node

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type fakeTicker struct {
	ch      chan time.Time
	polls   atomic.Int64
	stopped bool
	mu      sync.Mutex
}

func newFakeTicker() *fakeTicker { return &fakeTicker{ch: make(chan time.Time)} }
func (t *fakeTicker) C() <-chan time.Time {
	t.polls.Add(1)
	return t.ch
}
func (t *fakeTicker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}
func (t *fakeTicker) isStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

type fakePermitHelper struct {
	lang    string
	done    chan struct{}
	mu      sync.Mutex
	granted int
	closed  bool
}

func newFakeHelper(lang string) *fakePermitHelper {
	return &fakePermitHelper{lang: lang, done: make(chan struct{})}
}

func (h *fakePermitHelper) Grant() error {
	h.mu.Lock()
	h.granted++
	closed := h.closed
	h.closed = true
	h.mu.Unlock()
	if !closed {
		close(h.done)
	}
	return nil
}

func (h *fakePermitHelper) Done() <-chan struct{} { return h.done }

type permitRig struct {
	mu       sync.Mutex
	perms    proto.Permissions
	now      time.Time
	ticker   *fakeTicker
	opened   []string
	openErr  error
	helpers  []*fakePermitHelper
	startErr error
	pushes   chan proto.Permissions
	probeFn  func(context.Context, string) error
	mgr      *permitManager
}

func newPermitRig(t *testing.T, ctx context.Context) *permitRig {
	t.Helper()
	rig := &permitRig{
		perms:  proto.Permissions{LocalNetwork: false, Incoming: true, FullDisk: false},
		now:    time.Unix(1700000000, 0),
		ticker: newFakeTicker(),
		pushes: make(chan proto.Permissions, 16),
	}
	ops := &permitOps{
		probeAll: func() proto.Permissions {
			rig.mu.Lock()
			defer rig.mu.Unlock()
			return rig.perms
		},
		openSettings: func(target string) error {
			rig.mu.Lock()
			defer rig.mu.Unlock()
			rig.opened = append(rig.opened, target)
			return rig.openErr
		},
		startHelper: func(_ context.Context, lang string) (permitHelper, error) {
			rig.mu.Lock()
			defer rig.mu.Unlock()
			if rig.startErr != nil {
				return nil, rig.startErr
			}
			h := newFakeHelper(lang)
			rig.helpers = append(rig.helpers, h)
			return h, nil
		},
		probeFolder: func(pctx context.Context, folder string) error {
			if rig.probeFn != nil {
				return rig.probeFn(pctx, folder)
			}
			return nil
		},
		now: func() time.Time {
			rig.mu.Lock()
			defer rig.mu.Unlock()
			return rig.now
		},
		newTicker: func(time.Duration) permitTicker { return rig.ticker },
	}
	rig.mgr = newPermitManager(ctx, ops, func() {
		rig.pushes <- *rig.mgr.State()
	}, func(_ string, fn func()) { go fn() })
	return rig
}

func (r *permitRig) tick(t *testing.T) {
	t.Helper()
	waitCond(t, func() bool { return r.ticker.polls.Load() >= 1 })
	before := r.ticker.polls.Load()
	r.ticker.ch <- r.now
	waitCond(t, func() bool { return r.ticker.polls.Load() > before || r.ticker.isStopped() })
}

func TestPermitWatchTransitionsHelperAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rig := newPermitRig(t, ctx)

	if len(rig.pushes) != 0 {
		t.Fatalf("initial Probe pushed %d times", len(rig.pushes))
	}
	if got := *rig.mgr.State(); got != (proto.Permissions{LocalNetwork: false, Incoming: true, FullDisk: false}) {
		t.Fatalf("initial state = %+v", got)
	}

	// Unchanged re-probe pushes nothing.
	if err := rig.mgr.Handle(ctx, proto.HostPermit{}); err != nil {
		t.Fatalf("Handle empty = %v", err)
	}
	if len(rig.pushes) != 0 {
		t.Fatalf("unchanged Handle pushed %d times", len(rig.pushes))
	}

	// Advance 10s, unchanged tick pushes nothing and leaves watch running.
	rig.mu.Lock()
	rig.now = rig.now.Add(10 * time.Second)
	rig.mu.Unlock()
	rig.tick(t)
	if rig.ticker.isStopped() || len(rig.pushes) != 0 {
		t.Fatalf("unchanged tick stopped=%v pushes=%d", rig.ticker.isStopped(), len(rig.pushes))
	}

	// Transition local_network -> true pushes once; repeated tick does not.
	rig.mu.Lock()
	rig.perms.LocalNetwork = true
	rig.now = rig.now.Add(time.Second)
	rig.mu.Unlock()
	rig.tick(t)
	gotPush := <-rig.pushes
	if !gotPush.LocalNetwork || gotPush.FullDisk {
		t.Fatalf("push = %+v", gotPush)
	}
	rig.tick(t)
	if len(rig.pushes) != 0 {
		t.Fatalf("duplicate tick pushed %d times", len(rig.pushes))
	}

	// At 3 minutes, watch stops when no helper runs.
	rig.mu.Lock()
	rig.now = time.Unix(1700000000, 0).Add(3 * time.Minute)
	rig.mu.Unlock()
	rig.tick(t)
	waitCond(t, rig.ticker.isStopped)

	// Start helper (full_disk) in zh-CN; second call starts no second helper.
	rig.ticker = newFakeTicker()
	if err := rig.mgr.Handle(ctx, proto.HostPermit{Open: "full_disk", Lang: "zh-CN"}); err != nil {
		t.Fatalf("open full_disk = %v", err)
	}
	if err := rig.mgr.Handle(ctx, proto.HostPermit{Open: "full_disk", Lang: "en"}); err != nil {
		t.Fatalf("second open full_disk = %v", err)
	}
	rig.mu.Lock()
	helperCount := len(rig.helpers)
	firstLang := rig.helpers[0].lang
	rig.mu.Unlock()
	if helperCount != 1 || firstLang != "zh-CN" {
		t.Fatalf("helpers = %d lang=%q", helperCount, firstLang)
	}

	// Watch does not end past 3 minutes while helper runs.
	rig.mu.Lock()
	rig.now = rig.now.Add(10 * time.Minute)
	rig.mu.Unlock()
	rig.tick(t)
	if rig.ticker.isStopped() || len(rig.pushes) != 0 {
		t.Fatalf("watch stopped=%v pushes=%d while helper still open", rig.ticker.isStopped(), len(rig.pushes))
	}

	// When full_disk turns true, node pushes host.info, grants helper, and watch stops once helper closes.
	rig.mu.Lock()
	rig.perms.FullDisk = true
	rig.mu.Unlock()
	rig.tick(t)
	gotPush = <-rig.pushes
	if !gotPush.FullDisk {
		t.Fatalf("full_disk push = %+v", gotPush)
	}
	rig.helpers[0].mu.Lock()
	granted := rig.helpers[0].granted
	rig.helpers[0].mu.Unlock()
	if granted != 1 {
		t.Fatalf("helper granted = %d, want 1", granted)
	}
	waitCond(t, func() bool {
		select {
		case rig.ticker.ch <- rig.now:
		default:
		}
		return rig.ticker.isStopped()
	})

	// Cancelling node ctx stops an active watch cleanly.
	rig.ticker = newFakeTicker()
	if err := rig.mgr.Handle(ctx, proto.HostPermit{Open: "incoming"}); err != nil {
		t.Fatalf("open incoming = %v", err)
	}
	if !reflect.DeepEqual(rig.opened, []string{"incoming"}) {
		t.Fatalf("opened = %v", rig.opened)
	}
	cancel()
	waitCond(t, rig.ticker.isStopped)
}

func TestPermitErrorsAndFolderTimeout(t *testing.T) {
	rig := newPermitRig(t, t.Context())
	rig.mgr.folderWait = 20 * time.Millisecond

	if err := validatePermitRequest(proto.HostPermit{Open: "screen"}); err == nil || err.(*host.Error).Code != "bad_request" {
		t.Fatalf("validatePermitRequest(screen) = %v", err)
	}

	rig.openErr = errors.New("open failed")
	if err := rig.mgr.Handle(t.Context(), proto.HostPermit{Open: "local_network"}); !errors.Is(err, rig.openErr) {
		t.Fatalf("open error = %v", err)
	}
	rig.startErr = errors.New("helper failed")
	if err := rig.mgr.Handle(t.Context(), proto.HostPermit{Open: "full_disk"}); !errors.Is(err, rig.startErr) {
		t.Fatalf("startHelper error = %v", err)
	}

	// Folder timeout produces exact message; other errors pass through.
	rig.probeFn = func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	err := rig.mgr.CheckFolderTimeout("/Users/me/Documents")
	var wireErr *host.Error
	if !errors.As(err, &wireErr) || wireErr.Code != "permission" || wireErr.Message != "macOS is waiting for an answer about /Users/me/Documents on this computer" {
		t.Fatalf("CheckFolderTimeout = %v", err)
	}

	wantErr := errors.New("probe err")
	rig.probeFn = func(context.Context, string) error { return wantErr }
	if err := rig.mgr.CheckFolderTimeout("/Users/me/Documents"); !errors.Is(err, wantErr) {
		t.Fatalf("CheckFolderTimeout error = %v", err)
	}
}
