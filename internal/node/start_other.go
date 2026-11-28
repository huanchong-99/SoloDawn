//go:build !darwin

package node

func startBackground(exe string, _ bool) error {
	return detachNode(exe, "node")
}
