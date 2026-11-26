---
description: "Run your own SoloDawn rendezvous service (signalling, STUN, relay and the web client in one program), point nodes and browsers at it, update it, and protect it with the firewalls, tunnels and habits that the program itself does not provide."
kind: tutorial
---
# Self-host a rendezvous and protect it

## Summary

SoloDawn needs a small public service, the **rendezvous**, so that your computers and browsers can find each other. The official one is only a convenience: the rendezvous is one program file, `solodawn rendezvous`, the same `solodawn` binary that runs on your computers. This page shows how to run your own, how to point nodes and browsers at it, and how to protect it. Hand it to your AI: every command below is meant to be run by an assistant on a Linux server that you own.

The rendezvous never sees what you and your AIs say. Devices sign in to it with their keys, it passes small connection messages between them, it tells each device its public address (STUN), and it relays encrypted packets when no direct path works (TURN). It has no database, no accounts and no configuration file. The relay carries data that is already encrypted end to end; the rendezvous sees who connects, from which address, when, and how many bytes the relay moved.

The program defends only itself against ordinary misuse (limits listed in [What the program does](#what-the-program-does-and-does-not-do)). Keeping strangers away from it, from your computers and from your data is done outside the program, in [Protect it from the outside](#protect-it-from-the-outside).

## Prerequisites

- A server that your devices can reach (with a public IP address for setups B and C below), Linux with systemd, and root access. Size it to your own use.
- Two DNS names that both point to that server, for example `rv.example.org` (web client and signalling) and `turn.example.org` (relay over TLS). They must be different names. Add both A records, and both AAAA records if the server has IPv6.
- Ports the server can open: TCP 443, UDP 3478, and UDP 20000 to 29999 (the relay). Which ones you open depends on your setup; see [Choose your setup](#choose-your-setup) and [Ports](#ports).
- A release binary of `solodawn` for the server (see [install.md](../../install.md)), checked against `SHA256SUMS`; a binary built from source does not serve the web client or `/install.md`, which the onboarding prompt links. Use the same release on the server and on every computer, because the server also hands out the web client that is built into the binary.
- With your own certificate (setups A and B): a certificate and key (PEM) that cover the names you use.
- A name that you will keep. A node keeps the rendezvous address it was paired with, and version 1 has no command that changes it (see step 5), so choose a domain name, not an address, before you pair anything.

## Choose your setup

Choose before you start. The setup decides the flags in step 2, the ports and their sources in step 3, and which checks in step 6 apply.

| Setup | Best for | Flags (step 2) | Ports to open (step 3), from where |
| --- | --- | --- | --- |
| **A. Everyone on one VPN** (Tailscale or WireGuard) | One person with several devices, a small team | `--no-relay`, and `--cert` and `--key` | TCP 443 and UDP 3478, from the VPN only |
| **B. Public server, only your addresses allowed** | Fixed home or office addresses | `--cert` and `--key` | TCP 443, UDP 3478 and UDP 20000-29999, from your addresses only |
| **C. Public server, everyone allowed** | Running a service for others | none (automatic certificates); add the measures in [Additional measures for setup C](#additional-measures-for-setup-c) | TCP 443, UDP 3478 and UDP 20000-29999, from everywhere |

Recommended: **A**. Home and mobile addresses change, which makes **B** a chore. In **A** you need only one DNS name (no relay name) and no UDP range; devices in a VPN are expected to reach each other directly, which the program does not itself ensure. Browsers only run the page's cryptography on an HTTPS page or on `localhost`, so a rendezvous inside a VPN still needs a certificate and a name: with `--cert` and `--key` (for Tailscale, `tailscale cert` makes one for a `*.ts.net` name), or with a private ACME server (`--acme-directory`). The exact steps of these options with SoloDawn have not been tried. Automatic certificates need TCP 443 open to the whole internet, which is why only setup C uses them.

## Steps

### 1. Install the binary

Download the release binary for the server (`linux_amd64` or `linux_arm64`) and `SHA256SUMS` to an empty directory and run only the checksum part of [install.md](../../install.md)'s "Verify and install" block (not its `install` line, which targets a user's `~/.local/bin`), then:

```sh
sudo install -m 755 solodawn_<version>_linux_<arch> /usr/local/bin/solodawn
solodawn version
```

`solodawn version` must print the release tag you downloaded.

### 2. Create the service account and the unit

The unit below is for **setup C**. For A or B, change `ExecStart` as shown under it before you start the service.

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin solodawn
sudo tee /etc/systemd/system/solodawn-rendezvous.service >/dev/null <<'EOF'
[Unit]
Description=SoloDawn rendezvous
After=network-online.target
Wants=network-online.target

[Service]
User=solodawn
CacheDirectory=solodawn
Environment=XDG_CACHE_HOME=/var/cache
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes
ExecStart=/usr/local/bin/solodawn rendezvous --domain rv.example.org --turn-domain turn.example.org
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```

Replace the two names. Why these lines: the program writes certificates under the user's cache directory, which `XDG_CACHE_HOME=/var/cache` and `CacheDirectory=solodawn` turn into `/var/cache/solodawn/certs`, owned by the service account. `AmbientCapabilities` lets a non-root account listen on port 443. The program always ends with an error message (it never exits cleanly), so `Restart=always` is right.

Setups A and B:

- **Your own certificate** (setups A and B, where port 443 is not open to the whole internet): add `--cert /etc/solodawn/fullchain.pem --key /etc/solodawn/privkey.pem` to `ExecStart`. The certificate must cover the names you use (both names with the relay), and the `solodawn` account must be able to read both files. The program reads them once at start: after you renew them, run `sudo systemctl restart solodawn-rendezvous`, which drops relayed connections.
- **No relay** (setup A): replace `--turn-domain ...` with `--no-relay`. The server then offers signalling and STUN only, and you need no UDP range.
- **A private ACME server** (such as a step-ca of your own): add `--acme-directory <url>` and, if its HTTPS certificate is not publicly trusted, `--acme-ca <pem file>`.

### 3. Open the ports

On the server's firewall and in the cloud security group, allow in only what your setup needs, from the sources your setup allows:

| Protocol and port | For | Setups |
| --- | --- | --- |
| TCP 443 | web client, signalling, relay over TLS | A, B, C |
| UDP 3478 | STUN, relay over UDP | A, B, C |
| UDP 20000-29999 | relay sockets | B, C |
| TCP 22 | your administration | your own address only |

The source is the VPN (A), your own addresses (B) or everyone (C). With automatic certificates, Let's Encrypt must also reach TCP 443 from its own servers when it issues and renews. Those addresses are not fixed. So with automatic certificates, TCP 443 must be open to the whole internet, which is why setups A and B use `--cert` and `--key` instead (and you renew the certificate yourself). The ACME check is made over TLS on port 443 (it adds the `acme-tls/1` protocol), not over port 80, so port 80 is not needed.

### 4. Start it

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now solodawn-rendezvous
sudo systemctl status solodawn-rendezvous
```

The service is `active (running)`. A healthy server prints nothing at start; it logs only a line `relay: N allocations refused since start` (at most once a minute, when the number changes) and its errors. With automatic certificates, the first connection to each name makes the program ask the ACME server for that name's certificate, which takes a few seconds.

By starting it with automatic certificates you accept the certificate authority's terms of service (the program accepts them for you).

### 5. Point nodes and browsers at it

- **Browsers.** Open `https://rv.example.org/`. The page signs in to the server it came from (`wss://rv.example.org/rv`), so a browser uses your server by loading its page from your server. A browser pairs through the onboarding prompt that this page shows; the prompt already holds your server's address.
- **Computers (nodes).** Pair with `--rendezvous`:

  ```sh
  solodawn pair <secret> --name "<label>" --rendezvous wss://rv.example.org/rv
  ```

  `<secret>` is the pairing code that the onboarding page of your server shows (valid for 10 minutes). The prompt on that page gives the code and the address `--rendezvous wss://rv.example.org/rv`; it is not the whole command. The node saves that address as its list of rendezvous servers and uses it from then on. A node has no built-in address, so a node only knows a server because pairing gave it one.

A paired node keeps the addresses it was paired with. Version 1 has no command that changes them, and pairing again with `--rendezvous` is refused once the node has an owner. So the address you pair with is the one the node keeps: choose a name that you will keep (a domain, not an address) before you pair, and keep it pointing at the server.

All devices that should find each other must use the same rendezvous.

### 6. Check that it works

Run these from your own computer, not on the server and not by the assistant working on it (replace the names). In setup A run them from a device inside the VPN, in setup B from an allowed address.

```sh
# 1. The web client loads and the certificate is valid (a release binary also serves /install.md).
curl -sS -o /dev/null -w '%{http_code}\n' https://rv.example.org/            # 200

# 2. The relay name has a certificate too (TURN over TLS answers on the same port). Skip it with --no-relay.
openssl s_client -connect turn.example.org:443 -servername turn.example.org </dev/null 2>/dev/null | openssl x509 -noout -subject -dates

# 3. Signalling accepts a WebSocket and sends a challenge.
curl -sS --http1.1 -i --max-time 3 -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' https://rv.example.org/rv
# HTTP/1.1 101 Switching Protocols, then a frame holding {"t":"challenge","nonce":"..."}
# curl then waits for the closing frame and ends with exit 28 after 3 seconds; that is expected.

# 4. STUN answers on UDP 3478 (a binding response starts with 0101).
python3 - <<'EOF'
import os, socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM); s.settimeout(3)
s.sendto(bytes([0, 1, 0, 0, 0x21, 0x12, 0xA4, 0x42]) + os.urandom(12), ("rv.example.org", 3478))
print(s.recvfrom(512)[0][:2].hex())
EOF
```

The last proof is a real pairing, which needs the pairing code that the browser's onboarding page shows. `solodawn pair <code> --rendezvous wss://rv.example.org/rv` keeps waiting until the browser finishes pairing the node (it gives up after 10 minutes) and succeeds only then; an error within seconds means the node could not reach the server. The relay itself is proved only by two devices on networks that block direct paths; see [the real-machine checklist](i11-real-machine-checklist.md).

### 7. Check that the protection works

Run the checks of step 6 from an address that your setup does not allow (for example a phone on mobile data, outside the VPN): in setups A and B the four must fail, and from an allowed address the same four must succeed. In setup C there is nothing to refuse, so check the measures under [Additional measures for setup C](#additional-measures-for-setup-c) instead. Then from the same disallowed address check a node, if it has a public address: TCP and UDP 47800 and TCP 47801 of the node must not connect (for example `nc -vz -w 3 <node address> 47800`).

Also check that:

- the list of computers in the web client holds only computers you know;
- you have a backup of each group you care about (see Backups under [For every setup](#for-every-setup));
- you know what you will do first if a device is lost (see "A lost or stolen device" there).

## Update

A new release replaces the program file; nothing else changes, because the server keeps no data besides its certificates.

```sh
# After downloading and checking the new release as in install.md:
sudo systemctl stop solodawn-rendezvous
sudo install -m 755 solodawn_<new-version>_linux_<arch> /usr/local/bin/solodawn
sudo systemctl start solodawn-rendezvous
solodawn version
```

Direct connections that are already open do not pass through the server and stay up; relayed connections drop, and nodes reconnect by themselves. A restart resets the daily relay counters (they live in memory) and makes the server forget the relay credentials it issued; devices ask for new ones. Update your computers to the same release.

## Options, ports and files

### Options

`solodawn rendezvous` takes these options and nothing else. It reads no environment variable of its own and no configuration file. (`XDG_CACHE_HOME` and `HOME` only decide where the certificate cache goes.)

| Option | Default | Meaning |
| --- | --- | --- |
| `--domain <name>` | none | Signalling and STUN name. Turns on TLS, STUN and (unless `--no-relay`) the relay. Without it the server is plain HTTP and offers no STUN or relay. |
| `--turn-domain <name>` | none | Name for TURN over TLS. Required with `--domain` unless `--no-relay`, and must differ from `--domain`. |
| `--no-relay` | off | STUN and signalling only. Needs `--domain`. |
| `--listen <address>` | `:443` with `--domain`, otherwise `127.0.0.1:8080` | TCP address for signalling, the web client and (with `--domain`) relay over TLS. |
| `--cert <file>`, `--key <file>` | none | Your own PEM certificate and key for both names. Give both or neither; they need `--domain`. Without them the program gets certificates from an ACME server. |
| `--acme-directory <url>` | Let's Encrypt | ACME server to use. |
| `--acme-ca <file>` | system roots | PEM roots used to check the ACME server's HTTPS, and for nothing else. |
| `--udp-port <n>` | `3478` | UDP port for STUN and TURN. |
| `--relay-ports <first-last>` | `20000-29999` | UDP ports for relay sockets. Both ends 1 to 65535, in order. A narrower range allows fewer simultaneous relayed connections; the firewall must match. |
| `--relay-quota-mb <n>` | `50` | Relay allowance per device per UTC day, in MiB, both directions. `0` relays nothing. |

`--listen` with an address also decides where the UDP port binds: `:443` binds all addresses, `10.0.0.5:443` binds UDP 3478 on `10.0.0.5` only. The address in the relay's `turns:` URL uses the port of `--listen`, so a non-443 port is advertised as such. A wrong option prints the message and exits with status 2; a start-up failure (name that does not resolve, port in use, unreadable certificate) prints the error and exits with status 1.

### Ports

| Port | Protocol | Carries |
| --- | --- | --- |
| 443 (the `--listen` port) | TCP, TLS | `https://<domain>/` web client, `wss://<domain>/rv` signalling, and TURN over TLS (RFC 7443): the TURN name or ALPN `stun.turn` goes to the relay, any other connection is routed by its server name |
| 3478 (`--udp-port`) | UDP | STUN, TURN over UDP |
| 20000-29999 (`--relay-ports`) | UDP | one socket per relayed connection; only UDP is relayed, never TCP |

The server name must resolve (the program looks up `--domain` at start-up for the relay address). Behind a cloud NAT where the server's own interface does not hold the public address, the relay address advertised is the one the name resolves to; this arrangement is not tested.

A **node** listens on TCP and UDP 47800 (signalling and a local web page) and TCP 47801 (a direct fallback), on all addresses; it takes a free port if one is taken. It asks no router to open ports. Browsers open `http://localhost:47800/` only on the computer that runs the node (`solodawn open`).

### Files

| Where | What |
| --- | --- |
| The binary | The whole service, including the web client and `install.md` of its release |
| `<cache dir>/solodawn/certs/` (`/var/cache/solodawn/certs/` with the unit above; the user's cache directory otherwise: `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows) | Certificates and the ACME account key that the certificate library keeps. Only with automatic certificates |
| Nothing else | No log file (output goes to the journal), no database, no state |

## What the program does and does not do

Version 1 protects your content and your authority (device keys, signed messages, encrypted data channels, the group's home enforcing roles) and leaves the network around it to you.

What the rendezvous does itself:

- A device proves it holds its key (a challenge signature) before it is listed; a second sign-in of the same device replaces the first.
- Frames are at most 16 KiB, at most 120 in 10 seconds per connection, queues are bounded per receiver; a connection that breaks the rules is closed.
- On the TLS port, at most 256 open connections per IPv4 address or IPv6 /64, a 10-second handshake limit, header and idle timeouts.
- The relay hands out time-limited credentials, at most 32 allocations per device and 128 per source address, 64 peer addresses per client, and the daily byte quota per device. It refuses peers on private, carrier-grade NAT, loopback, link-local (cloud metadata) and other special ranges, and does not send to the server's own addresses (except its own relay sockets). It relays UDP only.
- The relay sees only ciphertext.

What it does **not** do:

- It does not restrict **who** may sign in. Any key that answers the challenge is accepted, and anyone can make a new key. So its limits slow down a single client but do not cap total traffic. The daily relay quota counts per device key, not per server.
- It does not limit total connections or relayed traffic across many source addresses, and does not detect a flood or a scan. In plain-HTTP mode (no `--domain`) the 256-connection limit does not apply.
- It does not protect you from a rendezvous you do not trust. Whoever runs it can see which devices connect from which addresses and when, can refuse, delay or drop messages, and can pretend to be offline. Version 1 has no defence against a rendezvous that tampers with connection set-up. Use your own or one you trust.
- It keeps no audit log of connections.
- It cannot refuse relay traffic to a NAT64 prefix that your own network chose (only the standard ranges are listed); use the server's outbound firewall for that.
- Nodes do not defend against scans or floods of their own ports 47800 and 47801, and version 1 has no way to revoke a lost device (revoking was postponed to after version 1). A group member, and the computer that runs a group (its home), is trusted: the home decides roles and which requests reach your AIs.

## Protect it from the outside

The setup you chose in [Choose your setup](#choose-your-setup) decides who can get close to the server. The rest is the same for all setups.

### For every setup

- **Do not expose a node's ports.** Do not forward 47800 or 47801 in a router, do not put a node in a router's DMZ, and keep the host firewall's default of refusing inbound connections (fine in setup A; in B and C it costs the direct paths, and on macOS allow `solodawn` in the Application Firewall as install.md says). The node asks the router for nothing, so a port is open to the internet only if you opened it. Where direct connections need an inbound path, allow them only from your VPN or your own addresses (TCP and UDP 47800, TCP 47801).
- **Computers.** Turn on full-disk encryption and a screen lock, and give each system account to one person. The data folder holds the device identity (`device.key`), the group data (`node.db`); do not put it in a synchronised folder or share it. It is `~/.local/share/solodawn` on Linux, `~/Library/Application Support/solodawn` on macOS and `%LOCALAPPDATA%\solodawn` on Windows. Anyone who can act as you on that computer can drive the node through its local socket.
- **A lost or stolen device.** Version 1 cannot revoke a device, so cut it off from the network instead: remove it from the VPN (Tailscale: delete the machine; WireGuard: delete its public key from every peer) or remove its address from the security group and firewalls. Then sign out, at their own websites, of the services you used on it (your code host, if you use one, and each AI command-line tool): SoloDawn never stores those credentials, but they sit on that disk. If it was the home of a group, restore the group from a backup on another computer (see Backups below); without a backup, create the group again. The lost device stays on your device list until revoking exists.
- **Groups.** Join only groups of people you trust, and only trust the computer that is a group's home with the group's content: it stores the group, decides who is a member and which requests reach your AIs. Send invitation links privately and to the person they are for (a link admits one person for 10 minutes). When a member leaves, remove them in the group and also from the VPN or the security group; what they have already read or copied cannot be taken back.
- **AI folders.** An AI works in the folder you chose for it, on your computer, as you and at full permission, and runs whatever commands it decides on, including code it wrote. SoloDawn runs no checks and makes no copy of the folder: your own uncommitted work there is within the AI's reach. Give it only folders you can afford to have changed, and commit or set aside your own changes first. For stricter isolation run the node as a separate system account or in a virtual machine with nothing else on it, and give the AI folders there.
- **Backups.** A group exists only on its home. Back up with `solodawn group export <group id> <file>` as [install.md](../../install.md) shows, and keep the file encrypted: it is as sensitive as the group. To restore on another computer of the same owner, run `solodawn group import <file>` there.
- **Server basics.** Keep the server's SSH to your own address and key-only, apply operating-system updates, and keep nothing else on it. The certificate key in the cache directory (or your `--key`) must stay readable only by the service account.
- **Browsers on other people's devices.** Pair a browser only on a device you trust; a paired browser is a device.

### Additional measures for setup C

- Put the relay in a network that cannot reach your own: the program already refuses private, carrier-grade NAT, loopback and link-local peers, and does not send to its own addresses, but add an outbound firewall rule for your own provider's NAT64 prefix and any other range you care about.
- Run nothing else on the server and attach it to no internal network.
- Set a bandwidth cap and a traffic alert at your cloud provider. The quota is per device key and anyone can create keys, so the program does not bound your bill.
- Lower `--relay-quota-mb` or `--relay-ports` to the capacity you can pay for.

## Failure and recovery

| Symptom | Cause and fix |
| --- | --- |
| Start-up error `relay needs a distinct --turn-domain` | Give `--turn-domain` a name other than `--domain`, or use `--no-relay`. |
| `--domain is required for TLS and STUN` | `--cert`, `--turn-domain` or `--no-relay` was given without `--domain`. |
| `--cert and --key must be supplied together` | Give both files or neither. |
| Error naming a lookup of the domain | The name does not resolve from the server. Fix DNS. |
| `bind: permission denied` on 443 | The unit lacks `AmbientCapabilities=CAP_NET_BIND_SERVICE`, or a non-root user runs it by hand. |
| `bind: address already in use` | Another program holds 443 or UDP 3478. Stop it, or use `--listen` and `--udp-port` (devices find the relay port from the server, but the firewall must match). |
| Browser warns about the certificate, or step 6 check 1 or 2 fails | Both names must resolve to the server and, with automatic certificates, TCP 443 must be open to the internet. Look at `journalctl -u solodawn-rendezvous`. A cached certificate lives in `/var/cache/solodawn/certs`; deleting the folder makes the program ask for new ones on the next connection. |
| Web client loads but nodes cannot pair | A node that has not finished pairing was given another address (or none): pair it again with `--rendezvous` of this server. A node that is already paired keeps the address it was paired with and cannot be moved in version 1 (see step 5, and `issues.md`). |
| Direct paths work, relayed connections do not | UDP 3478 or the relay range is blocked, or the quota is spent (it resets at 00:00 UTC). |
| `relay: N allocations refused since start` in the journal | Limits or credentials refused a request; normal in small numbers. A fast-growing number means someone is probing the relay: narrow who may reach it. |

## References

- [Rendezvous contract](../../planning/contracts/rendezvous.md): wire protocol, relay limits and section 6, self-hosting.
- [Rendezvous module](../../internal/rendezvous/README.md): how the service is built.
- [install.md](../../install.md): installing, updating and backing up a node.
- [Real-machine checklist](i11-real-machine-checklist.md): proving direct and relayed paths on real networks.
