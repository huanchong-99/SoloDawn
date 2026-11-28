# Flow: pairing and the device list

Primitives are in `contracts/envelope.md` (pairing secret, `pair` message,
device log) and `contracts/rendezvous.md` (rooms). Rules: R-138, R-152.

Pairing always happens between one browser and one node. The browser offers
the connection.

## A. First run: a browser meets its first node

Actors: owner, browser (web client), the owner's AI on the computer, node,
rendezvous.

1. The browser has no key. It generates a device key and stores it.
2. The browser creates a pairing secret, authenticates to the rendezvous
   service, and joins the room derived from the secret.
3. The onboarding page shows a prompt that contains the address of
   `install.md` and the pairing secret, with a 10-minute countdown. When the
   page's origin is not the official rendezvous, the prompt also gives
   `--rendezvous <url>` with the page's own signalling URL, `wss://<host>/rv`.
4. The owner copies the prompt to the AI on the computer to be added.
5. The AI follows `install.md`: download the release for this system, check
   its SHA-256, run `solodawn pair <secret> --name <label>`, with
   `--rendezvous <url>` when the prompt gives one.
6. `solodawn pair` starts the node process if it is not running and hands it
   the secret over the local socket. On a Mac, from the app, it starts the
   node as the login agent (`contracts/local.md` section 11). The node
   generates its device key on first start. A node that already belongs to a
   person refuses: the command says so and points to `solodawn pair-browser`
   (section C).
7. The node authenticates to the rendezvous service, the one the prompt
   names if it names one, and joins the room. The service sends `joined` to
   both sides.
8. Both sides send their `pair` message and verify the other's `mac`. The
   page shows the node's label and fingerprint, which its `pair` message
   carries; the node's version arrives with `welcome` and its system with
   `host.info` (steps 10 and 11).
9. The browser sends a signed `offer` to the node; the node answers; ICE
   runs (`flows/connect.md`); the `ctl` channel opens.
10. Until the node has adopted the person, the connection carries only
    `hello` and `devlog`. The browser writes log entry 1 (genesis, itself)
    and entry 2 (`add` the node) and sends both in a `devlog` frame. The
    node, which belongs to nobody yet, adopts that person because the log
    is valid and the entry that adds the node is signed by the browser at
    the other end of this pairing connection (`contracts/envelope.md`
    section 6). The browser sends `hello` again and the new `welcome` says
    `owner: true`.
11. The node registers itself to start at login (on a Mac it already is,
    from step 6), then reports the AI CLIs it found on the machine in
    `host.info` (`contracts/datachannel.md` section 7).
12. For each CLI the owner picks an account type (R-50); the browser sends
    `agent.set`. The page moves on to "create your first group".
13. `solodawn pair` prints the result (label, fingerprint, path in use) and
    exits 0. The AI reports it to the owner.

Failure at any step shows the step and the reason on the page and in the
command's output. A browser and node that found each other but could not
connect say so, and point at a firewall on the computer or its network. The secret is discarded when pairing succeeds, fails, or
times out.

## B. Adding another computer

Same as A from step 2, started from "add a computer" in the web client. At
step 10 the browser appends one `add` entry for the new node and sends the
whole log in a `devlog` frame. The browser then sends the new entry, the
same way, to the owner's other nodes and to the homes it is connected to.

Add one device at a time. Two devices of the same person that each add a
device at the same moment write two different entries for the same version
(`contracts/envelope.md` section 4).

## C. Adding another browser

Used for a phone, a second browser, or after site data was cleared.

1. On any of the owner's nodes: `solodawn pair-browser` creates a secret,
   joins the room, and prints a link
   `https://<web client>/#pair=<secret>` (no QR code). From the
   web client the owner can ask a node to do the same and display the link.
2. The new browser opens the link, generates its key, joins the room.
3. `pair` messages are exchanged and verified. The browser offers; `ctl`
   opens.
4. The node appends an `add` entry for the browser, signed by the node, and
   sends the whole log to the browser in a `devlog` frame. The browser,
   which belongs to nobody yet, adopts that person because the entry that
   adds it is signed by the node it paired with. It sends `hello` again,
   reads `owner: true`, and now knows the owner's other nodes.
5. The node sends the new entry, the same way, to the homes and the owner's
   browsers it is connected to. Those browsers pass it on to the owner's
   other nodes (`contracts/envelope.md` section 4); until one has, those
   nodes refuse the new browser (`risks.md` K-24).

`solodawn open` on a node does the same for the local browser in one step: it
creates a secret and opens `http://localhost:47800/#pair=<secret>`. The page
uses the node's own handler, where the node joins the room
(`contracts/rendezvous.md` section 1, "Local delivery").

A browser that has lost its key comes back this way too (`risks.md` K-20
says how a key is lost and what the client does against it). It has no
identity left: it shows the onboarding page and is paired as a new device,
and the owner revokes its old entry (section D). Its onboarding prompt,
given to the AI on a computer that is already set up, ends at step A.6:
`solodawn pair` refuses there and points to `solodawn pair-browser`.

## D. Revoking a device

Postponed after version 1: no screen sends a `revoke`.

1. From any of the owner's browsers, the owner chooses a device and
   confirms.
2. That browser appends a `revoke` entry and sends it in a `devlog` frame to
   every node of the owner and every home it is connected to.
3. Each receiver that accepts the entry refuses new connections from the
   revoked device (`unknown_device`). A node closes an open connection only
   for a revoked device of a group member, not for one of the owner's.
4. Receivers that were offline get the entry when they next connect to a
   peer that has it.

## What each party ends up knowing

| Party | Knows |
| --- | --- |
| browser | its key; the owner's device log; signalling addresses of the owner's nodes |
| node | its key; the owner's device log |
| rendezvous | during pairing: a room id and two device ids; afterwards nothing |
