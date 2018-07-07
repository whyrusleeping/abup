package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/urfave/cli"

	//github.com/zalando/go-keyring // this looks neat for password management

	bserv "github.com/ipfs/go-ipfs/blockservice"
	"github.com/ipfs/go-ipfs/core/coreunix"
	"github.com/ipfs/go-ipfs/exchange/bitswap"
	bsnet "github.com/ipfs/go-ipfs/exchange/bitswap/network"
	dag "github.com/ipfs/go-ipfs/merkledag"
	pin "github.com/ipfs/go-ipfs/pin"
	none "gx/ipfs/QmWLQyLU7yopJnwMvpHM5VSMG4xmbKgcq6P246mDy9xy5E/go-ipfs-routing/none"
	ipld "gx/ipfs/QmWi2BYBL5gJ3CiAiQchg6rn1A8iBsrWy51EYxvHVjFvLb/go-ipld-format"
	ma "gx/ipfs/QmYmsdtJ3HsodkePE3eU3TsCaP2YvPZJ4LoXnNkDE5Tpt7/go-multiaddr"
	libp2p "gx/ipfs/QmZ86eLPtXkQ1Dfa992Q8NpXArUoWWh3y728JDcWvzRrvC/go-libp2p"
	"gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	pstore "gx/ipfs/QmZR2XWVVBCtbgBWnQhWk2xcQfaR3W8faQPriAiaaj7rsr/go-libp2p-peerstore"
	cid "gx/ipfs/QmapdYm1b22Frv3k17fqrBYTFRxwiaVJkB299Mfn33edeB/go-cid"
	host "gx/ipfs/Qmb8T6YBBsjYsVGfrihQLfCJveczZnneSBqBKkYEBWDjge/go-libp2p-host"
	hamt "gx/ipfs/QmbasjzUYSMFwyFYRu6EX8727FrTTAGNUEGoM76uqPAERK/go-hamt-ipld"
	"gx/ipfs/QmdE4gMduCKCGAcczM2F5ioYDfdeKuPix138wrES1YSr7f/go-ipfs-cmdkit/files"
	peer "gx/ipfs/QmdVrMn1LhB4ybb8hMVaMLXnA8XRSewMnK6YqXKXoTcRvN/go-libp2p-peer"
	bstore "gx/ipfs/QmdpuJBPBZ6sLPj9BQpn3Rpi38BT2cF1QMiUfyzNWeySW4/go-ipfs-blockstore"
	crypto "gx/ipfs/Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5/go-libp2p-crypto"
	ds "gx/ipfs/QmeiCcJfDW1GJnWUArudsv5rQsihpi4oyddPhdqo3CfX6i/go-datastore"
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
	route, _ := none.ConstructNilRouting(nil, nil, nil, nil)
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

		key := []byte("this is my key")
		adder.FileWrapper = func(r io.Reader) (io.Reader, error) {
			return encryptReader(key, r)
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
		if _, err := os.Stat("config"); err == nil {
			return fmt.Errorf("already initialized!")
		}

		sc := ServerConfig{}
		fi, err := os.Create("config")
		if err != nil {
			return err
		}
		defer fi.Close()

		if err := json.NewEncoder(fi).Encode(sc); err != nil {
			return err
		}

		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return err
		}

		identkey, err := os.Create("identity.key")
		if err != nil {
			return err
		}
		defer identkey.Close()

		skb, err := priv.Bytes()
		if err != nil {
			return err
		}

		if _, err := identkey.Write(skb); err != nil {
			return err
		}

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
		initServerCommand,
	}
	app.RunAndExitOnError()
}
