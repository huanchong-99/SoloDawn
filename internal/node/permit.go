package node

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	permitWatchDuration = 3 * time.Minute
	permitWatchInterval = time.Second
	folderAccessTimeout = 2 * time.Second
)

var validOpenTargets = map[string]bool{
	"":              true,
	"local_network": true,
	"incoming":      true,
	"folders":       true,
	"full_disk":     true,
}

type (
	permitTicker interface {
		C() <-chan time.Time
		Stop()
	}
	permitHelper interface {
		Grant() error
		Done() <-chan struct{}
	}
	permitOps struct {
		probeAll     func() proto.Permissions
		openSettings func(target string) error
		startHelper  func(ctx context.Context, lang string) (permitHelper, error)
		probeFolder  func(ctx context.Context, folder string) error
		now          func() time.Time
		newTicker    func(time.Duration) permitTicker
	}
	// Lock order: probeMu -> mu; push and h.Grant run with neither lock held.
	permitManager struct {
		ctx        context.Context
		ops        permitOps
		push       func()
		spawn      func(name string, fn func())
		folderWait time.Duration
		probeMu    sync.Mutex
		mu         sync.Mutex
		state      proto.Permissions
		watching   bool
		watchUntil time.Time
		helper     permitHelper
	}
)

func newPermitManager(ctx context.Context, ops *permitOps, push func(), spawn func(string, func())) *permitManager {
	m := &permitManager{
		ctx:        ctx,
		ops:        *ops,
		push:       push,
		spawn:      spawn,
		folderWait: folderAccessTimeout,
	}
	m.Probe()
	return m
}

func validatePermitRequest(req proto.HostPermit) error {
	if !validOpenTargets[req.Open] {
		return &host.Error{Code: "bad_request", Message: "unknown permission target: " + req.Open}
	}
	return nil
}

// Probe runs all three permission checks once without pushing host.info.
func (p *permitManager) Probe() {
	p.probeOnce(false)
}

// State returns the latest permission snapshot.
func (p *permitManager) State() *proto.Permissions {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state
	return &s
}

// CheckFolderTimeout checks folder access for host.join and CLI starts,
// refusing after folderAccessTimeout when macOS is still waiting on the user.
func (p *permitManager) CheckFolderTimeout(folder string) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.folderWait)
	defer cancel()
	err := p.ops.probeFolder(ctx, folder)
	if errors.Is(err, context.DeadlineExceeded) {
		return &host.Error{Code: "permission", Message: "macOS is waiting for an answer about " + folder + " on this computer"}
	}
	return err
}

// Handle serves host.permit requests after surface validation and path resolution.
func (p *permitManager) Handle(ctx context.Context, req proto.HostPermit) error {
	if req.Path != "" {
		return p.ops.probeFolder(ctx, req.Path)
	}
	return p.handleStatus(req)
}

func (p *permitManager) handleStatus(req proto.HostPermit) error {
	if err := p.openAction(req.Open, req.Lang); err != nil {
		return err
	}
	p.probeOnce(true)
	p.ensureWatch()
	return nil
}

func (p *permitManager) probeOnce(notify bool) proto.Permissions {
	p.probeMu.Lock()
	next := p.ops.probeAll()
	changed, h := p.applyProbe(next)
	p.probeMu.Unlock()

	if next.FullDisk && h != nil {
		_ = h.Grant()
	}
	if notify && changed {
		p.push()
	}
	return next
}

func (p *permitManager) applyProbe(next proto.Permissions) (bool, permitHelper) {
	p.mu.Lock()
	defer p.mu.Unlock()
	changed := p.state != next
	p.state = next
	return changed, p.helper
}

func (p *permitManager) openAction(target, lang string) error {
	switch target {
	case "":
		return nil
	case "full_disk":
		return p.ensureHelper(lang)
	default:
		return p.ops.openSettings(target)
	}
}

func (p *permitManager) ensureHelper(lang string) error {
	p.mu.Lock()
	if p.helper != nil {
		p.mu.Unlock()
		return nil
	}
	h, err := p.ops.startHelper(p.ctx, lang)
	if err != nil {
		p.mu.Unlock()
		return err
	}
	p.helper = h
	p.mu.Unlock()
	p.spawn("permit_helper", func() {
		<-h.Done()
		p.mu.Lock()
		p.helper = nil
		p.mu.Unlock()
	})
	return nil
}

func (p *permitManager) ensureWatch() {
	p.mu.Lock()
	p.watchUntil = p.ops.now().Add(permitWatchDuration)
	start := !p.watching
	p.watching = true
	p.mu.Unlock()
	if start {
		p.spawn("permit_watch", p.runWatch)
	}
}

func (p *permitManager) runWatch() {
	ticker := p.ops.newTicker(permitWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			p.stopWatch()
			return
		case <-ticker.C():
			p.probeOnce(true)
			if p.shouldStopWatch() {
				return
			}
		}
	}
}

func (p *permitManager) shouldStopWatch() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.helper == nil && !p.ops.now().Before(p.watchUntil) {
		p.watching = false
		return true
	}
	return false
}

func (p *permitManager) stopWatch() {
	p.mu.Lock()
	p.watching = false
	p.mu.Unlock()
}
