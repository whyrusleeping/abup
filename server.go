package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	bserv "github.com/ipfs/go-ipfs/blockservice"
	"github.com/ipfs/go-ipfs/exchange/bitswap"
	bsnet "github.com/ipfs/go-ipfs/exchange/bitswap/network"
	dag "github.com/ipfs/go-ipfs/merkledag"
	inet "gx/ipfs/QmPjvxTpVH8qJyQDnxnsxF9kv9jezKD1kozz1hs3fCGsNh/go-libp2p-net"
	none "gx/ipfs/QmWLQyLU7yopJnwMvpHM5VSMG4xmbKgcq6P246mDy9xy5E/go-ipfs-routing/none"
	ipld "gx/ipfs/QmWi2BYBL5gJ3CiAiQchg6rn1A8iBsrWy51EYxvHVjFvLb/go-ipld-format"
	libp2p "gx/ipfs/QmZ86eLPtXkQ1Dfa992Q8NpXArUoWWh3y728JDcWvzRrvC/go-libp2p"
	cid "gx/ipfs/QmapdYm1b22Frv3k17fqrBYTFRxwiaVJkB299Mfn33edeB/go-cid"
	host "gx/ipfs/Qmb8T6YBBsjYsVGfrihQLfCJveczZnneSBqBKkYEBWDjge/go-libp2p-host"
	hamt "gx/ipfs/QmbasjzUYSMFwyFYRu6EX8727FrTTAGNUEGoM76uqPAERK/go-hamt-ipld"
	logging "gx/ipfs/QmcVVHfdyv15GVPk7NrxdWjh2hLVccXnoD8j2tyQShiXJb/go-log"
	cbor "gx/ipfs/QmcWjuCqrfAdtwChsnpHe9L1gvdYToCy8xLDY2KCdiTQiZ/go-ipld-cbor"
	bstore "gx/ipfs/QmdpuJBPBZ6sLPj9BQpn3Rpi38BT2cF1QMiUfyzNWeySW4/go-ipfs-blockstore"
	crypto "gx/ipfs/Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5/go-libp2p-crypto"
	ds "gx/ipfs/QmeiCcJfDW1GJnWUArudsv5rQsihpi4oyddPhdqo3CfX6i/go-datastore"
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
	route, _ := none.ConstructNilRouting(nil, nil, nil, nil)
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
