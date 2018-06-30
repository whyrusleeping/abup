package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	libp2p "gx/ipfs/QmNh1kGFFdsPu79KNSaL4NUKUPb4Eiz4KHdMtFY6664RDp/go-libp2p"
	host "gx/ipfs/QmNmJZL7FQySMtE2BQuLMuZg2EB2CLEunJJUSVSc9YnnbV/go-libp2p-host"
	cbor "gx/ipfs/QmRiRJhn427YVuufBEHofLreKWNw7P7BWNq86Sb9kzqdbd/go-ipld-cbor"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	ds "gx/ipfs/QmXRKBQA4wXP7xWbFiZsR1GP4HV6wMDQ1aWFxZZ4uBcPX9/go-datastore"
	inet "gx/ipfs/QmXfkENeeBvh3zYA51MaSdGUdBjhQ99cP5WQe8zgr6wchG/go-libp2p-net"
	none "gx/ipfs/QmXtoXbu9ReyV6Q4kDQ5CF9wXQNDY1PdHc4HhfxRR5AHB3/go-ipfs-routing/none"
	bstore "gx/ipfs/QmaG4DZ4JaqEfvPWt5nPPgoTzhc1tr1T3f4Nu9Jpdm8ymY/go-ipfs-blockstore"
	crypto "gx/ipfs/QmaPbCnUMBohSGo3KnxEa2bHqyJVVeEEcwtqJAYxerieBo/go-libp2p-crypto"
	bserv "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/blockservice"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap"
	bsnet "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap/network"
	dag "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/merkledag"
	hamt "gx/ipfs/QmcYBp5EDnJKfVN63F71rDTksvEf1cfijwCTWtw6bPG58T/go-hamt-ipld"
	cid "gx/ipfs/QmcZfnkapfECQGcLZaf9B79NRg7cRa9EnZh4LSbkCzwNvY/go-cid"
	ipld "gx/ipfs/Qme5bWv7wtjUNGsK2BNGVUFPKiuxWrsqrtvYwCLRw8YFES/go-ipld-format"
)

func init() {
	cbor.RegisterCborType(Record{})
	cbor.RegisterCborType(SyncRequest{})
	cbor.RegisterCborType(time.Time{})
}

var log = logging.Logger("abup")

type ServerNode struct {
	host  host.Host
	bswap *bitswap.Bitswap

	ds     ds.Datastore
	bstore bstore.Blockstore
	cstore *hamt.CborIpldStore
	dserv  ipld.DAGService

	head *cid.Cid
}

type Record struct {
	Prev *cid.Cid
	Val  *SyncRequest
}

func (sn *ServerNode) handleSync(s inet.Stream) {
	ctx := context.Background()
	defer s.Close()
	dec := json.NewDecoder(s)
	var sr SyncRequest

	if err := dec.Decode(&sr); err != nil {
		log.Error("reading sync request: ", err)
		return
	}

	if err := dag.FetchGraph(ctx, sr.NewObj, sn.dserv); err != nil {
		log.Error("failed to fetch graph: ", err)
		return
	}

	nrec := &Record{
		Prev: sn.head,
		Val:  &sr,
	}

	nhead, err := sn.cstore.Put(ctx, nrec)
	if err != nil {
		log.Error("failed to put new head:", err)
		return
	}

	sn.head = nhead

	resp := &SyncResponse{Msg: "OK"}
	if err := json.NewEncoder(s).Encode(resp); err != nil {
		log.Error("failed to write sync response: ", err)
		return
	}
}

func (sn *ServerNode) handleHead(s inet.Stream) {
	if err := json.NewEncoder(s).Encode(HeadResponse{sn.head}); err != nil {
		log.Error(err)
	}
}

type ServerConfig struct {
	ListenAddr string
}

func (sc *ServerConfig) GetListenAddr() string {
	if sc.ListenAddr != "" {
		return sc.ListenAddr
	}
	return "/ip4/0.0.0.0/tcp/7786"
}

func loadConfig(fname string) (*ServerConfig, error) {
	fi, err := os.Open(fname)
	if err != nil {
		return nil, err
	}

	var sc ServerConfig
	if err := json.NewDecoder(fi).Decode(&sc); err != nil {
		return nil, err
	}

	return &sc, nil
}

func loadKey(fname string) (crypto.PrivKey, error) {
	fi, err := os.Open(fname)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(fi)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(data)
}

func startServerNode(dir string) (*ServerNode, error) {
	conf, err := loadConfig(filepath.Join(dir, "config"))
	if err != nil {
		return nil, err
	}

	key, err := loadKey(filepath.Join(dir, "identity.key"))
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	opt := libp2p.ListenAddrStrings(conf.GetListenAddr())
	identopt := libp2p.Identity(key)
	h, err := libp2p.New(ctx, opt, identopt)
	if err != nil {
		return nil, err
	}

	mapds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(mapds)
	route, _ := none.ConstructNilRouting(nil, nil, nil)
	bnet := bsnet.NewFromIpfsHost(h, route)
	bswap := bitswap.New(ctx, bnet, bs)

	bserv := bserv.New(bs, bswap)
	dserv := dag.NewDAGService(bserv)

	cstore := &hamt.CborIpldStore{bserv}

	nr := &Record{}
	nrc, err := cstore.Put(ctx, nr)
	if err != nil {
		return nil, err
	}

	sn := &ServerNode{
		host:   h,
		bswap:  bswap.(*bitswap.Bitswap),
		ds:     mapds,
		bstore: bs,
		cstore: cstore,
		head:   nrc,
		dserv:  dserv,
	}

	h.SetStreamHandler(ProtocolSync, sn.handleSync)
	h.SetStreamHandler(ProtocolHead, sn.handleHead)

	return sn, nil
}
