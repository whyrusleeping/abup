package main

import (
	"context"
	"encoding/json"

	libp2p "gx/ipfs/QmNh1kGFFdsPu79KNSaL4NUKUPb4Eiz4KHdMtFY6664RDp/go-libp2p"
	host "gx/ipfs/QmNmJZL7FQySMtE2BQuLMuZg2EB2CLEunJJUSVSc9YnnbV/go-libp2p-host"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	ds "gx/ipfs/QmXRKBQA4wXP7xWbFiZsR1GP4HV6wMDQ1aWFxZZ4uBcPX9/go-datastore"
	none "gx/ipfs/QmXtoXbu9ReyV6Q4kDQ5CF9wXQNDY1PdHc4HhfxRR5AHB3/go-ipfs-routing/none"
	"gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	bstore "gx/ipfs/QmaG4DZ4JaqEfvPWt5nPPgoTzhc1tr1T3f4Nu9Jpdm8ymY/go-ipfs-blockstore"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap"
	bsnet "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap/network"
	cid "gx/ipfs/QmcZfnkapfECQGcLZaf9B79NRg7cRa9EnZh4LSbkCzwNvY/go-cid"
)

var log = logging.Logger("abup")

type ServerNode struct {
	host  host.Host
	bswap *bitswap.Bitswap

	ds     ds.Datastore
	bstore bstore.Blockstore

	head *cid.Cid
}

type Record struct {
	Prev *cid.Cid
}

func (sn *ServerNode) handleSync(s inet.Stream) {
	dec := json.NewDecoder(s)
	var sr SyncRequest

	if err := dec.Decode(&sr); err != nil {
		log.Error("reading sync request: ", err)
		return
	}
}

func (sn *ServerNode) handleHead(s inet.Stream) {
	if err := json.NewEncoder(s).Encode(HeadResponse{sn.head}); err != nil {
		log.Error(err)
	}
}

func startServerNode(dir string) (*ServerNode, error) {
	ctx := context.Background()
	opt := libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/7887")
	h, err := libp2p.New(ctx, opt)
	if err != nil {
		return err
	}

	mapds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(mapds)
	route := none.ConstructNilRouting(nil, nil, nil)
	bnet := bsnet.NewFromIpfsHost(h, route)
	bswap := bitswap.New(ctx, bnet)

	h.SetStreamHandler(ProtocolSync)
}
