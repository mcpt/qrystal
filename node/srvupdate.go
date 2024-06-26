package node

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cenkalti/rpc2"
	"github.com/nyiyui/qrystal/api"
	"github.com/nyiyui/qrystal/central"
	"github.com/nyiyui/qrystal/util"
)

func (n *Node) srvUpdate(cl *rpc2.Client, srvs []api.SRV) (err error) {
	for i, srv := range srvs {
		srvs[i].PeerName = n.cc.Networks[srv.NetworkName].Me
	}
	var s api.SRVUpdateS
	err = cl.Call("srvUpdate", &api.SRVUpdateQ{SRVs: srvs, CentralToken: n.cs.Token}, &s)
	if err != nil {
		err = fmt.Errorf("call: %w", err)
		return
	}
	// Since srvUpdate only propagates to peers that depend on this Node, and not this Node itself, we must propagate the change here ourselves.

	util.S.Infof("srv: called srvUpdate successfully")
	return
}

type SRVList struct {
	Networks map[string][]central.SRV
}

func (n *Node) loadSRVList(cl *rpc2.Client) (err error) {
	if n.srvListPath == "" {
		util.S.Infof("srv: blank srv list path, so not loading.")
		return nil
	}
	n.ccLock.Lock()
	defer n.ccLock.Unlock()
	util.S.Infof("srv: loading srv list from %s...", n.srvListPath)
	b, err := os.ReadFile(n.srvListPath)
	if err != nil {
		return fmt.Errorf("load list: %w", err)
	}
	var sl SRVList
	err = json.Unmarshal(b, &sl)
	if err != nil {
		return fmt.Errorf("load list: %w", err)
	}
	util.S.Infof("srv: loaded srv list: %#v", sl)
	srvs := make([]api.SRV, 0)
	for cnn, srvs2 := range sl.Networks {
		cn, ok := n.cc.Networks[cnn]
		if !ok {
			util.S.Warnf("srv list: network nonexistent: %s", cnn)
			continue
		}
		me := cn.Peers[cn.Me]
		me.SRVs = central.UpdateSRVs(me.SRVs, srvs2)
		for _, srv2 := range srvs2 {
			srvs = append(srvs, api.SRV{
				NetworkName: cnn,
				SRV:         srv2,
			})
		}
	}
	return n.srvUpdate(cl, srvs)
}
