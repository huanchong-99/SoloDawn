//go:build acceptance

package features

import (
	"context"
	"path/filepath"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (w *s3DriverWorld) remote() error {
	m := &m3World{f: w.f, cleanup: w.cleanup, tb: scenarioTB{TB: w.f.t, cleanup: w.cleanup}, groups: map[string]string{}, homeView: newHomeView()}
	root, err := shortTempRoot(w.f.t, w.cleanup, "s3remote")
	if err != nil {
		return err
	}
	m.root, m.repoSrc = root, filepath.Join(root, "src")
	if err := initLocalGitRepo(m.repoSrc); err != nil {
		return err
	}
	m.cluster = nodetest.Start(m.tb, nodetest.Config{Root: root, BinDir: w.f.fakeCLIs, Nodes: 2})
	hostNode := m.cluster.Nodes[1]
	m.activeHost = hostNode.Host
	sock, stop, err := attachMCPSocket(w.f.t, w.cleanup, hostNode.Host, filepath.Join(hostNode.Dir, "mcp"))
	if err != nil {
		return err
	}
	m.sockPath, m.closeMCP = sock, stop
	m.attachBrowser()
	w.caps.m3 = m
	return s3Run(func() error { return m.createGroup("Website") }, w.joinRemote, w.memory)
}

func (w *s3DriverWorld) joinRemote() error {
	m := w.caps.m3
	n := m.cluster.Nodes[1]
	link := node.AttachLocal(n.Host, m.node().Home)
	w.cleanup(func() { link.Close() })
	n.Host.Attach(m.node().Key.ID(), link)
	if _, err := n.Host.Agents(context.Background(), false); err != nil {
		return err
	}
	set := proto.AgentSet{HostCommand: newHostCmd(&m.ids, n.Key.ID(), "agent.set"), Agent: w.agent, Account: "api_key"}
	join := proto.HostJoin{HostCommand: newHostCmd(&m.ids, n.Key.ID(), "host.join"), Group: proto.ID(m.groups["Website"]), Home: m.node().Key.ID(), Agent: w.agent, Name: "Worker"}
	return s3Run(func() error { return callHostSurface(n.Host, link, m.cluster.Owner.ID(), "agent.set", set) },
		func() error { return callHostSurface(n.Host, link, m.cluster.Owner.ID(), "host.join", join) },
		func() error {
			return m.drainBrowserUntil(5*time.Second, func() bool { _, ok := m.aiByName["Worker"]; return ok }, nil)
		})
}
