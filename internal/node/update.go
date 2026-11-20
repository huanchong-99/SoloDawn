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

// UpdateStatus is the update field of host.info. It is nil before installation
// unless a downloaded binary failed verification.
type UpdateStatus struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// Updater is owned by the node. ReleaseURL is the latest release's download
// directory (releases/latest/download on GitHub). Binary is the installed path.
// The owner serializes Check and Apply, and serializes Apply with admission
// of turns and takeovers. Only Check uses the network.
type Updater struct {
	ReleaseURL string
	Binary     string
	Version    string
	Busy       func() bool
	Restart    func() error
	Status     *UpdateStatus

	nextCheck time.Time
	ready     *releaseFile
}

type releaseFile struct {
	name, version, sum string
	staged             string
}

var errUpdateMismatch = errors.New("self-update: SHA-256 mismatch")

// Check reads the latest manifest once a day. A verified download waits in
// the installation directory until Apply can install it.
func (u *Updater) Check(ctx context.Context, now time.Time) error {
	if u.ready != nil || now.Before(u.nextCheck) {
		return nil
	}
	u.nextCheck = now.Add(24 * time.Hour)
	body, err := u.open(ctx, "SHA256SUMS")
	if err != nil {
		return err
	}
	defer body.Close()
	file, err := findRelease(body, updateTarget(runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	if !newerRelease(file.version, u.Version) {
		u.Status = nil
		return nil
	}
	return u.prepare(ctx, file)
}

func (u *Updater) open(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(u.ReleaseURL, "/")+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("self-update: %s: %s", name, resp.Status)
	}
	return resp.Body, nil
}

func updateTarget(goos, arch string) string {
	target := goos + "_" + arch
	if goos == "windows" {
		target += ".exe"
	}
	return target
}

const versionPattern = `(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*)){2}`

var releaseVersion = regexp.MustCompile(fmt.Sprintf(`^v?%s$`, versionPattern))

func findRelease(body io.Reader, target string) (*releaseFile, error) {
	line := regexp.MustCompile(`^([[:xdigit:]]{64}) [ *](solodawn_v?(` + versionPattern + `)_` + regexp.QuoteMeta(target) + `)$`)
	var file *releaseFile
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		m := line.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		if file != nil {
			return nil, fmt.Errorf("self-update: multiple SHA256SUMS entries for %s", target)
		}
		file = &releaseFile{name: m[2], version: m[3], sum: m[1]}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("self-update: no valid SHA256SUMS entry for %s", target)
	}
	return file, nil
}

// Compare the three decimal components of stable releases without integer
// overflow. A development build has no release version to advance.
func newerRelease(version, current string) bool {
	if !releaseVersion.MatchString(current) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(current, "v"), ".")
	order := slices.CompareFunc(strings.Split(version, "."), parts, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b))
	})
	return order > 0
}

func (u *Updater) prepare(ctx context.Context, file *releaseFile) error {
	body, err := u.open(ctx, file.name)
	if err != nil {
		return err
	}
	defer body.Close()
	file.staged, err = stageRelease(filepath.Dir(u.Binary), body, file.sum)
	if errors.Is(err, errUpdateMismatch) {
		u.Status = &UpdateStatus{Version: file.version, Status: "mismatch"}
		log.Printf("%v for %s", err, file.name)
	}
	if err != nil {
		return err
	}
	u.Status = nil
	u.ready = file
	return nil
}

func stageRelease(dir string, body io.Reader, sum string) (string, error) {
	f, err := os.CreateTemp(dir, ".solodawn-update-*")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), body)
	if err := errors.Join(copyErr, f.Close()); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), sum) {
		os.Remove(f.Name())
		return "", errUpdateMismatch
	}
	return f.Name(), nil
}

// Apply is called after Check and when session activity changes. Busy must
// include every working or takeover session. Restart requests the handoff to
// the newly installed binary. A failed restart can be tried again when idle.
func (u *Updater) Apply() error {
	if u.ready == nil || u.Busy() {
		return nil
	}
	if u.ready.staged != "" {
		if err := installRelease(u.ready.staged, u.Binary, runtime.GOOS); err != nil {
			return err
		}
		u.ready.staged = ""
		u.Status = &UpdateStatus{Version: u.ready.version, Status: "pending"}
	}
	if err := u.Restart(); err != nil {
		return err
	}
	u.ready = nil
	return nil
}

func installRelease(staged, binary, goos string) error {
	info, err := os.Stat(binary)
	if err != nil {
		return err
	}
	if err := os.Chmod(staged, info.Mode().Perm()); err != nil {
		return err
	}
	if goos != "windows" {
		return os.Rename(staged, binary)
	}
	// Windows permits moving the running executable aside, but not
	// overwriting it. The previous image can be overwritten after restart.
	old := binary + ".old"
	if err := os.Rename(binary, old); err != nil {
		return err
	}
	if err := os.Rename(staged, binary); err != nil {
		return errors.Join(err, os.Rename(old, binary))
	}
	return nil
}
