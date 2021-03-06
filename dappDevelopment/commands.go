package dappDevelopment

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	sk "github.com/Bit-Nation/pangea-tools/signingKey"
	dapp "github.com/Bit-Nation/panthalassa/dapp"
	pb "github.com/Bit-Nation/panthalassa/dapp/registry/pb"
	lp2p "github.com/libp2p/go-libp2p"
	net "github.com/libp2p/go-libp2p-net"
	pui "github.com/manifoldco/promptui"
	qrterminal "github.com/mdp/qrterminal"
	ma "github.com/multiformats/go-multiaddr"
	protoMc "github.com/multiformats/go-multicodec/protobuf"
	fsw "github.com/radovskyb/watcher"
	cli "github.com/urfave/cli"
)

func config(cfg *lp2p.Config) error {
	addr, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	if err != nil {
		return err
	}
	cfg.ListenAddrs = []ma.Multiaddr{
		addr,
	}
	return lp2p.Defaults(cfg)
}

var DAppStream = cli.Command{
	Name:      "dapp:stream",
	ArgsUsage: "[DApp file path] [signing key path]",
	Action: func(c *cli.Context) error {

		// make sure dApp bundle exist
		dAppFileName := c.Args().Get(0)
		if _, err := os.Stat(dAppFileName); err != nil {
			fmt.Println("DApp file does not exist")
			return nil
		}

		// make sure signing key exist
		signingKey := c.Args().Get(1)
		if _, err := os.Stat(signingKey); err != nil {
			fmt.Println("signing key does not exist")
			return nil
		}

		// decrypt signing key
		content, err := ioutil.ReadFile(signingKey)
		if err != nil {
			panic(err)
		}

		// ask for password
		p := pui.Prompt{
			Label: "Enter password for signing key encryption",
			Mask:  '*',
		}
		pw, err := p.Run()
		if err != nil {
			panic(err)
		}
		sk, err := sk.Decrypt(content, []byte(pw))
		if err != nil {
			panic(err)
		}

		h, err := lp2p.New(context.Background(), lp2p.Defaults, config)
		if err != nil {
			panic(err)
		}

		h.SetStreamHandler("/dapp-development/0.0.0", func(stream net.Stream) {

			writer := bufio.NewWriter(stream)
			reader := bufio.NewReader(stream)
			protoEnc := protoMc.Multicodec(nil).Encoder(writer)
			protoDec := protoMc.Multicodec(nil).Decoder(reader)

			w := fsw.New()
			w.SetMaxEvents(1)
			w.FilterOps(fsw.Write)

			go func() {
				for {
					select {
					case <-w.Event:

						// read DApp
						content, err := ioutil.ReadFile(dAppFileName)
						if err != nil {
							panic(err)
							return
						}

						// unmarshal content
						dAppRep := dapp.JsonRepresentation{}
						if err := json.Unmarshal(content, &dAppRep); err != nil {
							panic(err)
							return
						}

						// sign DApp
						dAppRep.SignaturePublicKey = sk.PublicKey
						hashOfDApp, err := dAppRep.Hash()
						if err != nil {
							panic(err)
						}

						dAppSignature := sk.Sign(hashOfDApp)
						if err != nil {
							panic(err)
						}
						dAppRep.Signature = dAppSignature

						// marshal DApp representation
						rawDAppRep, err := dAppRep.Marshal()
						if err != nil {
							panic(err)
						}

						err = protoEnc.Encode(&pb.Message{
							Type: pb.Message_DApp,
							DApp: rawDAppRep,
						})
						if err != nil {
							panic(err)
						}

						if err := writer.Flush(); err != nil {
							panic(err)
							return
						}

					case err := <-w.Error:
						fmt.Println(err)
					case <-w.Closed:
						return
					}
				}
			}()

			go func() {

				for {

					msg := pb.Message{}
					if err := protoDec.Decode(&msg); err != nil {
						fmt.Println(err)
					}
					fmt.Println(string(msg.Log))

				}

			}()

			w.Add("dapp_build.json")
			w.Start(time.Millisecond * 100)

		})

		for _, addr := range h.Addrs() {
			a := fmt.Sprintf("%s/ipfs/%s", addr.String(), h.ID().Pretty())
			qrterminal.Generate(a, qrterminal.M, os.Stdout)
			fmt.Println(a, "\n\n\n")
		}

		select {}

		return nil
	},
}
