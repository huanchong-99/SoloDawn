// Run on native Windows: go run ./spikes/S-5/socket_windows.go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func must[T any](v T, err error) T {
	check(err)
	return v
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func dacl(path string) string {
	return must(windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)).String()
}

func setDACL(path, sddl string) {
	sd := must(windows.SecurityDescriptorFromString(sddl))
	acl, _, err := sd.DACL()
	check(err)
	check(windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.PROTECTED_DACL_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil))
}

func line(from, to net.Conn, want string) {
	_, err := io.WriteString(from, want+"\n")
	check(err)
	got := must(bufio.NewReader(to).ReadString('\n'))
	if got != want+"\n" {
		panic("line mismatch: " + got)
	}
	fmt.Print(got)
}

func main() {
	dir := must(os.MkdirTemp(must(os.UserConfigDir()), "s5-"))
	defer os.RemoveAll(dir)
	sid := must(windows.GetCurrentProcessToken().GetTokenUser()).User.Sid.String()
	setDACL(dir, "D:P(A;OICI;FA;;;"+sid+")")
	path := filepath.Join(dir, "sock")
	listen := func() *net.UnixListener {
		return must(net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"}))
	}
	l := listen()
	before := dacl(path)
	fmt.Println("sock DACL:", strings.ReplaceAll(before, sid, "<current-user>"))
	check(os.Chmod(path, 0600))
	fmt.Printf("chmod(0600): reported %04o; DACL unchanged: %t\n", must(os.Stat(path)).Mode().Perm(), before == dacl(path))

	client := must(net.DialTimeout("unix", path, 5*time.Second))
	check(l.SetDeadline(time.Now().Add(5 * time.Second)))
	server := must(l.AcceptUnix())
	for _, c := range []net.Conn{client, server} {
		check(c.SetDeadline(time.Now().Add(5 * time.Second)))
	}
	line(client, server, `{"cmd":"status"}`)
	line(server, client, `{"done":true,"ok":true}`)
	check(client.Close())
	check(server.Close())
	setDACL(path, "D:P(A;;FR;;;"+sid+")")
	c, err := net.DialTimeout("unix", path, time.Second)
	switch {
	case err == nil:
		check(c.Close())
		fmt.Println("read-only socket: connected")
	case errors.Is(err, windows.WSAEACCES), errors.Is(err, os.ErrPermission):
		fmt.Println("read-only socket: access denied")
	default:
		panic(err)
	}
	check(l.Close())
	_, err = os.Lstat(path)
	if !errors.Is(err, os.ErrNotExist) {
		panic(fmt.Sprintf("endpoint after close: %v", err))
	}
	check(listen().Close())
	fmt.Println("close removed sock; rebind OK")
}
