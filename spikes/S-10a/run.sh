#!/usr/bin/env bash
# LAB holds a disposable SoloDawn checkout in work/, a Go module cache in
# work/.s10a/mod/, pnpm in pnpm/, and the Playwright install in browsers/.
set -euo pipefail
lab=$(realpath "${LAB:?set LAB to the prepared experiment directory}")
cd "$lab/work"
work=$PWD
scratch=$work/.s10a
mkdir -p "$scratch"/{home,tmp,shm}
goroot=$(GOTOOLCHAIN=local go env GOROOT)
nodebin=$(dirname "$(command -v node)")
mode=$1
shift

case "$1" in
  go)
    command=(sh -ec 'go build -o .s10a/solodawn ./cmd/solodawn && .s10a/solodawn version && go test -count=1 ./...')
    ;;
  node)
    command=(sh -ec 'cd web && pnpm build && pnpm test')
    ;;
  probe)
    shift
    command=(python3 spikes/S-10a/probe.py child "$@")
    ;;
  *) exit 2 ;;
esac

case "$mode" in
  sandbox)
    wrap=(bwrap --unshare-all --die-with-parent --new-session --cap-drop ALL)
    for dir in /usr/{bin,include,lib,lib64,share/fonts} /etc/fonts \
               "$goroot" "$lab/pnpm" "$lab/browsers"; do
      wrap+=(--ro-bind "$dir" "$dir")
    done
    wrap+=(--ro-bind "$nodebin/node" "$nodebin/node"
           --symlink usr/bin /bin --symlink usr/lib /lib --symlink usr/lib64 /lib64
           --proc /proc --remount-ro /proc --symlink /proc/self/fd /dev/fd)
    for device in null zero random urandom; do
      wrap+=(--dev-bind "/dev/$device" "/dev/$device")
    done
    wrap+=(--bind "$work" "$work" --ro-bind "$work/.git" "$work/.git"
           --symlink "$scratch/tmp" /tmp --symlink "$scratch/shm" /dev/shm
           --remount-ro / --chdir "$work" --)
    ;;
  native) wrap=() ;;
  *) exit 2 ;;
esac

exec env -i PATH="$goroot/bin:$lab/pnpm/node_modules/.bin:$nodebin:/usr/bin:/bin" LANG=C.UTF-8 \
  HOME="$scratch/home" TMPDIR="$scratch/tmp" GOCACHE="$scratch/gocache" \
  GOMODCACHE="$scratch/mod" GOPROXY=off GOTOOLCHAIN=local \
  PLAYWRIGHT_BROWSERS_PATH="$lab/browsers" \
  "${wrap[@]}" "${command[@]}"
