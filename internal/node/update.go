package node

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// UpdateStatus is the update field of host.info (contracts/datachannel.md 7).
type UpdateStatus struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// Updater is owned by the node, which calls Check and Apply from one goroutine.
// ReleaseURL is the latest release's download directory (releases/latest/download
// on GitHub), Binary the installed executable, Version the running release tag.
// Busy reports whether a session is working or in takeover. Transport
// carries the requests; nil is http.DefaultTransport.
type Updater struct {
	ReleaseURL string
	Binary     string
	Version    string
	Busy       func() bool
	Restart    func() error
	Status     *UpdateStatus
	Transport  http.RoundTripper

	nextCheck time.Time
}

// Check reads the latest release once a day. A newer binary whose SHA-256
// matches SHA256SUMS replaces Binary and waits for Apply to restart; a
// mismatch is logged and kept in Status, and nothing is replaced.
func (u *Updater) Check(ctx context.Context, now time.Time) error {
	if u.pending() || now.Before(u.nextCheck) {
		return nil
	}
	u.nextCheck = now.Add(24 * time.Hour)
	name, tag, sum, err := u.latest(ctx)
	if err != nil {
		return err
	}
	u.Status = nil
	if newerRelease(tag, u.Version) {
		u.Status, err = u.update(ctx, name, tag, sum)
	}
	return err
}

func (u *Updater) latest(ctx context.Context) (name, tag, sum string, err error) {
	sums, err := u.get(ctx, "SHA256SUMS")
	if err != nil {
		return "", "", "", err
	}
	defer sums.Close()
	return findRelease(sums, updateTarget(runtime.GOOS, runtime.GOARCH))
}

func (u *Updater) update(ctx context.Context, name, tag, sum string) (*UpdateStatus, error) {
	body, err := u.get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	staged, err := stageRelease(filepath.Dir(u.Binary), body, sum)
	if errors.Is(err, errUpdateMismatch) {
		log.Printf("%v for %s", err, name)
		return &UpdateStatus{Version: tag, Status: "mismatch"}, nil
	}
	if err == nil {
		if err = installRelease(staged, u.Binary, runtime.GOOS); err != nil {
			os.Remove(staged)
		}
	}
	if err != nil {
		return nil, err
	}
	return &UpdateStatus{Version: tag, Status: "pending"}, nil
}

// Apply restarts into the installed release when no session is busy. The node
// calls it after Check and whenever a session leaves working or takeover.
func (u *Updater) Apply() error {
	if !u.pending() || u.Busy() {
		return nil
	}
	return u.Restart()
}

func (u *Updater) pending() bool {
	return u.Status != nil && u.Status.Status == "pending"
}

// Downloads are limited: a longer file is cut, and so fails its checksum.
var (
	maxDownload   int64         = 256 << 20
	updateTimeout time.Duration = 600e9 // 10 minutes
)

// maxRedirects allows GitHub's two (latest release, then its asset store).
const maxRedirects = 5

var errRedirect = errors.New("self-update: redirect to another scheme or too many redirects")

// get fetches a file of the release over https only, following at most
// maxRedirects redirects, each to https.
func (u *Updater) get(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.ReleaseURL+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("self-update: %s is not https", u.ReleaseURL)
	}
	client := &http.Client{Transport: u.Transport, Timeout: updateTimeout, CheckRedirect: httpsRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("self-update: %s: %s", name, resp.Status)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, maxDownload), resp.Body}, nil
}

func httpsRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || len(via) > maxRedirects {
		return errRedirect
	}
	return nil
}

func updateTarget(goos, arch string) string {
	target := goos + "_" + arch
	if goos == "windows" {
		target += ".exe"
	}
	return target
}

// findRelease returns the file, tag and SHA-256 of this target's line in SHA256SUMS.
func findRelease(sums io.Reader, target string) (name, tag, sum string, err error) {
	line := regexp.MustCompile(`^([0-9a-f]{64})  (solodawn_(v[^_]+)_` + regexp.QuoteMeta(target) + `)$`)
	scanner := bufio.NewScanner(sums)
	for scanner.Scan() {
		if m := line.FindStringSubmatch(scanner.Text()); m != nil {
			return m[2], m[3], m[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", "", err
	}
	return "", "", "", fmt.Errorf("self-update: no SHA256SUMS entry for %s", target)
}

var stableTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// newerRelease compares two vMAJOR.MINOR.PATCH tags by their three numbers. A
// pre-release tag is never newer, and a development build is never updated.
func newerRelease(tag, current string) bool {
	if !stableTag.MatchString(tag) || !stableTag.MatchString(current) {
		return false
	}
	byNumber := func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b)) }
	return slices.CompareFunc(stableTag.FindStringSubmatch(tag)[1:], stableTag.FindStringSubmatch(current)[1:], byNumber) > 0
}

var errUpdateMismatch = errors.New("self-update: SHA-256 mismatch")

// stageRelease downloads into the binary's folder, so that installing is a rename.
func stageRelease(dir string, body io.Reader, sum string) (string, error) {
	f, err := os.CreateTemp(dir, ".solodawn-update-*")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, hash), body)
	if err = errors.Join(err, f.Sync(), f.Close()); err == nil && hex.EncodeToString(hash.Sum(nil)) != sum {
		err = errUpdateMismatch
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func installRelease(staged, binary, goos string) error {
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}
	if goos != "windows" {
		return os.Rename(staged, binary)
	}
	// Windows lets a running executable be renamed, not replaced. The
	// previous image is overwritten by the next update, or put back when the
	// new one cannot take its place.
	old := binary + ".old"
	if err := os.Rename(binary, old); err != nil {
		return err
	}
	if err := os.Rename(staged, binary); err != nil {
		return errors.Join(err, os.Rename(old, binary))
	}
	return nil
}
