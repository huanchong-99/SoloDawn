package spike

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func browser(input []byte) error {
	cmd := exec.Command("node", "browser.cjs")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func probeClients(t *testing.T, secret string, mux *http.ServeMux) {
	pc := must(webrtc.NewPeerConnection(webrtc.Configuration{
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
		ICEServers:         []webrtc.ICEServer{issue(secret, "pion", time.Hour)},
	}))
	defer pc.Close()
	channel := must(pc.CreateDataChannel("ctl", nil))
	channel.OnMessage(func(msg webrtc.DataChannelMessage) { check(channel.SendText(string(msg.Data))) })
	gathered := webrtc.GatheringCompletePromise(pc)
	check(pc.SetLocalDescription(must(pc.CreateOffer(nil))))
	<-gathered
	mux.HandleFunc("POST /answer", func(w http.ResponseWriter, r *http.Request) {
		var answer webrtc.SessionDescription
		check(json.NewDecoder(r.Body).Decode(&answer))
		check(pc.SetRemoteDescription(answer))
		w.WriteHeader(http.StatusNoContent)
	})
	input := must(json.Marshal(map[string]any{"offer": pc.LocalDescription(), "ice": issue(secret, "browser", time.Hour)}))
	if err := browser(input); err != nil {
		t.Fatal(err)
	}
	pair := must(pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair())
	if pair == nil || pair.Local.Typ != webrtc.ICECandidateTypeRelay || pair.Remote.Typ != webrtc.ICECandidateTypeRelay {
		t.Fatalf("Pion pair %v", pair)
	}
	t.Logf("Pion local=%s remote=%s", pair.Local.Typ, pair.Remote.Typ)
}
