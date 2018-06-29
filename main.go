package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli"

	libp2p "gx/ipfs/QmNh1kGFFdsPu79KNSaL4NUKUPb4Eiz4KHdMtFY6664RDp/go-libp2p"
	host "gx/ipfs/QmNmJZL7FQySMtE2BQuLMuZg2EB2CLEunJJUSVSc9YnnbV/go-libp2p-host"
	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	ds "gx/ipfs/QmXRKBQA4wXP7xWbFiZsR1GP4HV6wMDQ1aWFxZZ4uBcPX9/go-datastore"
	none "gx/ipfs/QmXtoXbu9ReyV6Q4kDQ5CF9wXQNDY1PdHc4HhfxRR5AHB3/go-ipfs-routing/none"
	"gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	peer "gx/ipfs/QmZoWKhxUmZ2seW4BzX6fJkNR8hh9PsGModr7q171yq2SS/go-libp2p-peer"
	bstore "gx/ipfs/QmaG4DZ4JaqEfvPWt5nPPgoTzhc1tr1T3f4Nu9Jpdm8ymY/go-ipfs-blockstore"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/core/coreunix"
	"gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap"
	bsnet "gx/ipfs/QmcKwjeebv5SX3VFUGDFa4BNMYhy14RRaCzQP7JN3UQDpB/go-ipfs/exchange/bitswap/network"
	cid "gx/ipfs/QmcZfnkapfECQGcLZaf9B79NRg7cRa9EnZh4LSbkCzwNvY/go-cid"
	"gx/ipfs/QmceUdzxkimdYsgtX733uNgzf1DLHyBKN6ehGSp85ayppM/go-ipfs-cmdkit/files"
)

var _ = ma.LengthPrefixedVarSize

var ProtocolSync = protocol.ID("/abup/sync")

var ProtocolHead = protocol.ID("/abup/head")

type ClientNode struct {
	host   host.Host
	bswap  *bitswap.Bitswap
	bstore bstore.Blockstore

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

func startClientNode() (*ClientNode, error) {
	ctx := context.Background()
	h, err := libp2p.New(ctx)
	if err != nil {
		return nil, err
	}

	mapds := ds.NewMapDatastore()
	bs := bstore.NewBlockstore(mapds)
	route, _ := none.ConstructNilRouting(nil, nil, nil)
	bnet := bsnet.NewFromIpfsHost(h, route)
	bswap := bitswap.New(ctx, bnet, bs)

	return &ClientNode{
		host:   h,
		bswap:  bswap.(*bitswap.Bitswap),
		bstore: bs,
	}, nil
}

type SyncRequest struct {
	NewObj    *cid.Cid
	Path      string
	Host      string
	Timestamp time.Time
}

type SyncResponse struct {
	Msg string
}

var saveCommand = cli.Command{
	Name: "save",
	Action: func(c *cli.Context) error {
		arg := c.Args().First()
		dir, fname := filepath.Split(arg)

		finfo, err := os.Stat(arg)
		if err != nil {
			return err
		}

		fi, err := files.NewSerialFile(fname, arg, false, finfo)
		if err != nil {
			return err
		}

		adder, err := coreunix.NewAdder(context.Background(), pinner, bstore, dagserv)
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

		cn, err := startClientNode()
		if err != nil {
			return err
		}

		sr := &SyncRequest{
			NewObj:    nd.Cid(),
			Path:      arg,
			Host:      "mycomputersname",
			Timestamp: time.Now(),
		}

		if err := cn.PutSyncRequest(sr); err != nil {
			return err
		}

		return nil
	},
}

var listCommand = cli.Command{
	Name: "list",
	Action: func(c *cli.Context) error {
		cn, err := startClientNode()
		if err != nil {
			return err
		}

		return nil
	},
}

func main() {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		saveCommand,
	}
}
