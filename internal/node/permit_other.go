//go:build !darwin

package node

func newPermitOps(string) *permitOps {
	return nil
}
