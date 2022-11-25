package cs

import (
	"crypto/ed25519"
	"fmt"
	"log"

	"github.com/nyiyui/qrystal/central"
	"github.com/nyiyui/qrystal/node/api"
	"github.com/nyiyui/qrystal/util"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (s *CentralSource) convertCC(tokenNetworks map[string]string) (*api.CentralConfig, error) {
	s.ccLock.RLock()
	defer s.ccLock.RUnlock()
	cc := s.cc
	networks := map[string]*api.CentralNetwork{}
	for cnn, cn := range cc.Networks {
		me, ok := tokenNetworks[cnn]
		if !ok {
			continue
		}
		mePeer := cn.Peers[me]
		if mePeer == nil {
			panic("mePeer is nil")
		}
		peers := map[string]*api.CentralPeer{}
		for pn, peer := range cn.Peers {
			if mePeer.CanSee != nil {
				log.Printf("peer %s CanSee %v", me, mePeer.CanSee)
				if !contains(mePeer.CanSee.Only, pn) {
					continue
				}
			}
			peers[pn] = &api.CentralPeer{
				Host:            peer.Host,
				AllowedIPs:      FromIPNets(central.ToIPNets(peer.AllowedIPs)),
				ForwardingPeers: peer.ForwardingPeers,
				PublicKey: &api.PublicKey{
					Raw: []byte(peer.PublicKey),
				},
			}
		}
		networks[cnn] = &api.CentralNetwork{
			Ips:        FromIPNets(central.ToIPNets(cn.IPs)),
			Me:         me,
			Keepalive:  durationpb.New(cn.Keepalive),
			ListenPort: int32(cn.ListenPort),
			Peers:      peers,
		}
	}
	return &api.CentralConfig{
		Networks: networks,
	}, nil
}

func convertPeer(peer *api.CentralPeer) (*central.Peer, error) {
	allowedIPs, err := ToIPNets(peer.AllowedIPs)
	if err != nil {
		return nil, fmt.Errorf("AllowedIPs: %w", err)
	}
	if l := len(peer.PublicKey.Raw); l != ed25519.PublicKeySize {
		return nil, fmt.Errorf("PublicKey: invalid size %d b", l)
	}
	return &central.Peer{
		Host:       peer.Host,
		AllowedIPs: central.FromIPNets(allowedIPs),
		PublicKey:  util.Ed25519PublicKey(peer.PublicKey.Raw),
	}, nil
}
