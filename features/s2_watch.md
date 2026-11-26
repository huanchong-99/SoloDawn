# I2-1c acceptance runs

| Scenario | Units |
| --- | --- |
| A9.20 and A9.21 | P-4a, P-4b, T2-1a, T2-1b, T2-2a, HS-19, HS-20 and T2-6. I2-2 exercises the browser with U2-3 and U-11. |

## A9.21 selected-pair change

The watcher is a pion peer on 127.0.0.1 with the cast's TURN server from
the start. `moveToRelay` (`s2_watch_peer_test.go`) delays its direct
packets by 3 seconds, waits until ICE has measured that, and cuts direct
UDP; ICE renominates the relayed pair on the same PeerConnection. The
delay is needed: pion rates a pair by its last round trip even when it no
longer answers. The packets to the TURN server take 5 ms, as pion rates a
round trip of 0 as none measured, and on Windows a relay round trip on
127.0.0.1 reads 0. The browser and a real network are in the
[real-machine checklist](../docs/cookbook/i11-real-machine-checklist.md),
section 6.
