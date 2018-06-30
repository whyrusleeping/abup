package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/urfave/cli"

	libp2p "gx/ipfs/QmNh1kGFFdsPu79KNSaL4NUKUPb4Eiz4KHdMtFY6664RDp/go-libp2p"
	host "gx/ipfs/QmNmJZL7FQySMtE2BQuLMuZg2EB2CLEunJJUSVSc9YnnbV/go-libp2p-host"
	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	ds "gx/ipfs/QmXRKBQA4wXP7xWbFiZsR1GP4HV6wMDQ1aWFxZZ4uBcPX9/go-datastore"
	pstore "gx/ipfs/QmXauCuJzmzapetmC6W4TuDJLL1yFFrVzSHoWv8YdbmnxH/go-libp2p-peerstore"
	none "gx/ipfs/QmXtoXbu9ReyV6Q4kDQ5CF9wXQNDY1PdHc4HhfxRR5AHB3/go-ipfs-routing/none"
	"gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	peer "gx/ipfs/QmZoWKhxUmZ2seW4BzX6fJkNR8hh9PsGModr7q171yq2SS/go-libp2p-peer"
	bstore "gx/ipfs/QmaG4DZ4JaqEfvPWt5nPPgoTzhc1tr1T3f4Nu9Jpdm8ymY/go-ipfs-blockstore"
	bserv "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/blockservice"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/core/coreunix"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap"
	bsnet "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap/network"
	dag "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/merkledag"
	pin "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/pin"
	hamt "gx/ipfs/QmcYBp5EDnJKfVN63F71rDTksvEf1cfijwCTWtw6bPG58T/go-hamt-ipld"
	cid "gx/ipfs/QmcZfnkapfECQGcLZaf9B79NRg7cRa9EnZh4LSbkCzwNvY/go-cid"
	"gx/ipfs/QmceUdzxkimdYsgtX733uNgzf1DLHyBKN6ehGSp85ayppM/go-ipfs-cmdkit/files"
	ipld "gx/ipfs/Qme5bWv7wtjUNGsK2BNGVUFPKiuxWrsqrtvYwCLRw8YFES/go-ipld-format"
)

var _ = ma.LengthPrefixedVarSize

var ProtocolSync = protocol.ID("/abup/sync")

var ProtocolHead = protocol.ID("/abup/head")

type ClientNode struct {
	host   host.Host
	bswap  *bitswap.Bitswap
	bstore bstore.GCBlockstore
	ds     ds.Datastore
	dserv  ipld.DAGService
	cstore *hamt.CborIpldStore

	serverID peer.ID
}

type HeadResponse struct {
	Cid *cid.Cid
}

func (cn *ClientNode) Head() (*cid.Cid, error) {
	ctx := context.Background()
	s, err := cn.host.NewStream(ctx, cn.serverID, ProtocolHead)
	if err != nil {
		return nil, err
	}

	var hr HeadResponse
	if err := json.NewDecoder(s).Decode(&hr); err != nil {
		return nil, err
	}

	return hr.Cid, nil
}

func (cn *ClientNode) PutSyncRequest(sr *SyncRequest) error {
	ctx := context.Background()

	s, err := cn.host.NewStream(ctx, cn.serverID, ProtocolSync)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(s).Encode(sr); err != nil {
		return err
	}

	var resp SyncResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		return err
	}

	fmt.Println("SYNC RESPONSE: ", resp.Msg)
	return nil
}

func startClientNode(server string) (*ClientNode, error) {
	addr, err := ma.NewMultiaddr(server)
	if err != nil {
		return nil, err
	}

	pinfo, err := pstore.InfoFromP2pAddr(addr)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	h, err := libp2p.New(ctx)
	if err != nil {
		return nil, err
	}

	mapds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(mapds)
	gcbs := bstore.NewGCBlockstore(bs, bstore.NewGCLocker())
	route, _ := none.ConstructNilRouting(nil, nil, nil)
	bnet := bsnet.NewFromIpfsHost(h, route)
	bswap := bitswap.New(ctx, bnet, gcbs)
	dserv := dag.NewDAGService(bserv.New(gcbs, bswap))

	cstore := &hamt.CborIpldStore{bserv.New(bs, bswap)}

	if err := h.Connect(ctx, *pinfo); err != nil {
		return nil, err
	}

	return &ClientNode{
		host:     h,
		bswap:    bswap.(*bitswap.Bitswap),
		bstore:   gcbs,
		ds:       mapds,
		dserv:    dserv,
		cstore:   cstore,
		serverID: pinfo.ID,
	}, nil
}

type SyncRequest struct {
	NewObj    *cid.Cid
	Path      string
	Host      string
	Timestamp int64
}

type SyncResponse struct {
	Msg string
}

var saveCommand = cli.Command{
	Name: "save",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "server",
			EnvVar: "ABUP_SERVER",
		},
	},
	Action: func(c *cli.Context) error {
		arg := c.Args().First()
		_, fname := filepath.Split(arg)

		finfo, err := os.Stat(arg)
		if err != nil {
			return err
		}

		fi, err := files.NewSerialFile(fname, arg, false, finfo)
		if err != nil {
			return err
		}

		server := c.String("server")
		if server == "" {
			return fmt.Errorf("must pass server flag")
		}
		cn, err := startClientNode(server)
		if err != nil {
			return err
		}

		pinner := pin.NewPinner(cn.ds, cn.dserv, cn.dserv)

		adder, err := coreunix.NewAdder(context.Background(), pinner, cn.bstore, cn.dserv)
		if err != nil {
			return err
		}

		if err := adder.AddFile(fi); err != nil {
			return err
		}

		nd, err := adder.Finalize()
		if err != nil {
			return err
		}

		abspath, err := filepath.Abs(arg)
		if err != nil {
			return err
		}

		sr := &SyncRequest{
			NewObj:    nd.Cid(),
			Path:      abspath,
			Host:      "mycomputersname",
			Timestamp: time.Now().UnixNano(),
		}

		if err := cn.PutSyncRequest(sr); err != nil {
			return err
		}

		return nil
	},
}

var listCommand = cli.Command{
	Name: "list",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "server",
			EnvVar: "ABUP_SERVER",
		},
	},
	Action: func(c *cli.Context) error {
		cn, err := startClientNode(c.String("server"))
		if err != nil {
			return err
		}

		head, err := cn.Head()
		if err != nil {
			return err
		}

		for head != nil {
			var rec Record
			ctx := context.Background()
			if err := cn.cstore.Get(ctx, head, &rec); err != nil {
				return err
			}
			head = rec.Prev

			if rec.Val == nil {
				fmt.Println("<empty>")
				continue
			}
			fmt.Println(rec.Val.Path)
		}

		return nil
	},
}

var serveCommand = cli.Command{
	Name: "serve",
	Action: func(c *cli.Context) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		sn, err := startServerNode(cwd)
		if err != nil {
			panic(err)
		}

		for _, a := range sn.host.Addrs() {
			fmt.Printf("%s/ipfs/%s\n", a, sn.host.ID().Pretty())
		}
		select {}
	},
}

var initServerCommand = cli.Command{
	Name: "init-server",
	Action: func(c *cli.Context) error {
		return nil
	},
}

func main() {
	fi, _ := os.Create("profile")
	defer fi.Close()
	pprof.StartCPUProfile(fi)
	defer pprof.StopCPUProfile()

	app := cli.NewApp()
	app.Commands = []cli.Command{
		saveCommand,
		listCommand,
		serveCommand,
	}
	app.RunAndExitOnError()
}
