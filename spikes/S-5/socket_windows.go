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

func ownerSID() string {
	return must(windows.GetCurrentProcessToken().GetTokenUser()).User.Sid.String()
}

func listen(path, grants string) *net.UnixListener {
	l := must(net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"}))
	setDACL(path, "D:P(A;;FA;;;"+ownerSID()+")"+grants)
	return l
}

func dial(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, 5*time.Second)
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

func roundTrip() {
	dir := must(os.MkdirTemp(must(os.UserConfigDir()), "s5-"))
	defer os.RemoveAll(dir)
	sid := ownerSID()
	setDACL(dir, "D:P(A;OICI;FA;;;"+sid+")")
	path := filepath.Join(dir, "sock")
	l := listen(path, "")
	before := dacl(path)
	fmt.Println("sock DACL:", strings.ReplaceAll(before, sid, "<current-user>"))
	check(os.Chmod(path, 0600))
	fmt.Printf("chmod(0600): reported %04o; DACL unchanged: %t\n", must(os.Stat(path)).Mode().Perm(), before == dacl(path))

	client := must(dial(path))
	check(l.SetDeadline(time.Now().Add(5 * time.Second)))
	server := must(l.AcceptUnix())
	for _, c := range []net.Conn{client, server} {
		check(c.SetDeadline(time.Now().Add(5 * time.Second)))
	}
	line(client, server, `{"cmd":"status"}`)
	line(server, client, `{"done":true,"ok":true}`)
	check(client.Close())
	check(server.Close())
	check(l.Close())
	_, err := os.Lstat(path)
	if !errors.Is(err, os.ErrNotExist) {
		panic(fmt.Sprintf("endpoint after close: %v", err))
	}
	check(listen(path, "").Close())
	fmt.Println("close removed sock; rebind OK")
}

// Both paths are in the same traversable directory. Only the endpoint DACL differs.
func serve(other string) {
	if other == ownerSID() {
		panic("two distinct accounts required")
	}
	private := listen(must(filepath.Abs("sock")), "")
	defer private.Close()
	control := listen(must(filepath.Abs("control")), "(A;;FA;;;"+other+")")
	defer control.Close()
	fmt.Println("READY")
	check(control.SetDeadline(time.Now().Add(time.Minute)))
	c := must(control.Accept())
	defer c.Close()
	check(c.SetDeadline(time.Now().Add(10 * time.Second)))
	got := must(bufio.NewReader(c).ReadString('\n'))
	fmt.Print("second account: ", got)
	if got != "control connected; owner-only socket denied\n" {
		panic("owner-only isolation not demonstrated")
	}
}

func secondAccount(other string) {
	if ownerSID() != other {
		panic("client is not running as the test account")
	}
	control := must(dial(must(filepath.Abs("control"))))
	defer control.Close()
	conn, err := dial(must(filepath.Abs("sock")))
	if err == nil {
		check(conn.Close())
		_, err = fmt.Fprintln(control, "control connected; owner-only socket CONNECTED")
		check(err)
		panic("unexpected connection")
	}
	if !errors.Is(err, windows.WSAEACCES) && !errors.Is(err, os.ErrPermission) {
		panic(err)
	}
	_, err = fmt.Fprintln(control, "control connected; owner-only socket denied")
	check(err)
}

func main() {
	switch {
	case len(os.Args) == 1:
		roundTrip()
	case len(os.Args) == 3 && os.Args[1] == "serve":
		serve(os.Args[2])
	case len(os.Args) == 3 && os.Args[1] == "client":
		secondAccount(os.Args[2])
	default:
		panic("usage: socket [serve|client second-account-SID]")
	}
}
