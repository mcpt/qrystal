package node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/nyiyui/qanms/mio"
	"github.com/nyiyui/qanms/node/api"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type SyncRes struct {
	netStatus map[string]SyncNetRes
}

func (r *SyncRes) String() string {
	b := new(strings.Builder)
	for nn, ns := range r.netStatus {
		fmt.Fprintf(b, "net %s:\n%s\n", nn, &ns)
	}
	return b.String()
}

type SyncNetRes struct {
	err        error
	peerStatus map[string]SyncPeerRes
}

func (r *SyncNetRes) String() string {
	b := new(strings.Builder)
	if r.err != nil {
		fmt.Fprintf(b, "\terr: %s\n", r.err)
	} else {
		fmt.Fprint(b, "\tno error\n")
	}
	for pn, ps := range r.peerStatus {
		fmt.Fprintf(b, "\tpeer %s: %s\n", pn, &ps)
	}
	return b.String()
}

type SyncPeerRes struct {
	err    error
	config wgtypes.PeerConfig
}

func (r *SyncPeerRes) String() string {
	b := new(strings.Builder)
	fmt.Fprintf(b, "\t\terr: %s\n", r.err)
	return b.String()
}

func (c *Node) Sync(ctx context.Context) (*SyncRes, error) {
	res := SyncRes{
		netStatus: map[string]SyncNetRes{},
	}
	c.ccLock.RLock()
	defer c.ccLock.RUnlock()
	for cnn := range c.cc.Networks {
		log.Printf("===SYNCING net %s", cnn)
		netRes, err := c.syncNetwork(ctx, cnn)
		if netRes == nil {
			netRes = &SyncNetRes{}
		}
		netRes.err = err
		res.netStatus[cnn] = *netRes
	}
	return &res, nil
}

func (c *Node) syncNetworkWG(cn *CentralNetwork, peers []wgtypes.PeerConfig) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()
	cfg := wgtypes.Config{
		PrivateKey:   cn.myPrivKey,
		ListenPort:   &cn.ListenPort,
		ReplacePeers: true,
		Peers:        peers,
	}
	me := cn.Peers[cn.Me]
	err := c.mio.ConfigureDevice(mio.ConfigureDeviceQ{
		Name:    cn.name,
		Config:  &cfg,
		Address: ToIPNets(me.AllowedIPs),
		// TODO: fix to use my IPs
	})
	if err != nil {
		return fmt.Errorf("mio: %w", err)
	}
	return nil
}

func (c *Node) syncNetwork(ctx context.Context, cnn string) (*SyncNetRes, error) {
	cn := c.cc.Networks[cnn]
	err := ensureWGPrivKey(cn)
	if err != nil {
		return nil, errors.New("private key generation failed")
	}

	res := SyncNetRes{
		peerStatus: map[string]SyncPeerRes{},
	}
	peers := make([]wgtypes.PeerConfig, 0, len(cn.Peers))
	for pn := range cn.Peers {
		if pn == cn.Me {
			continue
		}
		/*
			// TODO: fix deadlock when syncing to each other
			if pn == "ko" {
				continue
			}
		*/
		log.Printf("syncing net %s peer %s", cn.name, pn)
		ps := c.syncPeer(ctx, cnn, pn)
		res.peerStatus[pn] = ps
		if ps.err == nil {
			peers = append(peers, ps.config)
		}
	}
	err = c.syncNetworkWG(cn, peers)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Node) syncPeer(ctx context.Context, cnn string, pn string) (res SyncPeerRes) {
	cn := c.cc.Networks[cnn]
	err := c.ensureClient(ctx, cnn, pn)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("ensure client: %w", err)}
	}
	log.Printf("preping")
	err = c.ping(ctx, cnn, pn)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("ping: %w", err)}
	}
	log.Printf("preauth")
	err = c.auth(ctx, cnn, pn)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("auth: %w", err)}
	}
	log.Printf("prexch")
	err = c.xch(ctx, cnn, pn)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("xch: %w", err)}
	}
	log.Printf("postxch")
	peer := cn.Peers[pn]
	peer.lock.RLock()
	defer peer.lock.RUnlock()
	hostOnly, _, err := net.SplitHostPort(peer.Host)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("SplitHostPort: %w", err)}
	}
	wgHost := fmt.Sprintf("%s:%d", hostOnly, cn.ListenPort)
	resolvedHost, err := net.ResolveUDPAddr("udp", wgHost)
	if err != nil {
		return SyncPeerRes{err: fmt.Errorf("resolve host: %w", err)}
	}
	keepalive := cn.Keepalive
	config := wgtypes.PeerConfig{
		PublicKey:                   *peer.pubKey,
		Remove:                      false,
		PresharedKey:                peer.psk,
		Endpoint:                    resolvedHost,
		PersistentKeepaliveInterval: &keepalive,
		ReplaceAllowedIPs:           true,
		AllowedIPs:                  ToIPNets(peer.AllowedIPs),
	}
	return SyncPeerRes{config: config}
}

func (c *Node) auth(ctx context.Context, cnn string, pn string) (err error) {
	c.serversLock.Lock()
	defer c.serversLock.Unlock()
	log.Print("servers", c.servers)
	cs, ok := c.servers[networkPeerPair{cnn, pn}]
	if !ok {
		return errors.New("corresponding clientServer not found")
	}
	log.Print("preauth")
	conn, err := cs.cl.Auth(ctx)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	log.Print("postinitauth")
	cn := c.cc.Networks[cnn]
	state := authState{
		coordPrivKey: c.coordPrivKey,
		conn:         conn,
		cc:           c.cc,
		cn:           cn,
		you:          cn.Peers[pn],
	}
	log.Print("preauthboth")
	err = state.authMine(cnn, pn)
	if err != nil {
		return fmt.Errorf("authenticating me: %w", err)
	}
	err = state.authOthers()
	if err != nil {
		return fmt.Errorf("authenticating you: %w", err)
	}

	sq, err := conn.Recv()
	if err != nil {
		return err
	}
	token := sq.Sq.(*api.AuthSQ_Token).Token.Token
	cs.token = string(token)
	return nil
}

func (c *Node) xch(ctx context.Context, cnn string, pn string) (err error) {
	cn := c.cc.Networks[cnn]
	peer := cn.Peers[pn]
	// TODO: dont xch if locked?
	peer.lsaLock.Lock()
	defer peer.lsaLock.Unlock()
	if time.Since(peer.lsa) < 1*time.Second {
		return errors.New("attempted to sync too recently")
	}
	peer.lock.Lock()
	defer peer.lock.Unlock()
	cs := c.servers[networkPeerPair{cnn, pn}]
	pubKey := c.cc.Networks[cnn].myPrivKey.PublicKey()
	psk, err := wgtypes.GenerateKey()
	if err != nil {
		return errors.New("PSK generation failed")
	}
	if cs.token == "" {
		return errors.New("blank token")
	}
	s, err := cs.cl.Xch(ctx, &api.XchQ{
		Token:  []byte(cs.token),
		PubKey: pubKey[:],
		Psk:    psk[:],
	})

	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	yourPubKey, err := wgtypes.NewKey(s.PubKey)
	if err != nil {
		return errors.New("invalid public key")
	}
	peer.pubKey = &yourPubKey
	peer.psk = &psk
	log.Println("SET1 PSK", peer, "same", psk)
	peer.latestSync = time.Now()
	return nil
}

func (c *Node) ping(ctx context.Context, cnn string, pn string) (err error) {
	cs := c.servers[networkPeerPair{cnn, pn}]
	_, err = cs.cl.Ping(ctx, &api.PingQS{})
	return
}
