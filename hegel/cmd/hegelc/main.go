package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/tinkerbell/hegel/grpc/protos/hegel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	server string
	port   int

	defaultServer = "metadata"
	defaultPort   = 50060

	envVarServer = "HEGEL_SERVER"
	envVarPort   = "HEGEL_PORT"
)

func main() {
	if envServer := os.Getenv(envVarServer); envServer != "" {
		defaultServer = envServer
	}
	if envPort := os.Getenv(envVarPort); envPort != "" {
		var err error
		if defaultPort, err = strconv.Atoi(envPort); err != nil {
			log.Panic(err)
		}
	}

	flag.StringVar(&server, "server", defaultServer, fmt.Sprintf("The hostname or address of the Hegel service [%s]", envVarServer))
	flag.IntVar(&port, "port", defaultPort, fmt.Sprintf("The port of the Hegel service [%s]", envVarPort))
	flag.Parse()

	config := &tls.Config{
		// TODO: Investigate whether it is safe to remove this dangerous default
		InsecureSkipVerify: true, //nolint:gosec // G402: TLS InsecureSkipVerify set true
	}

	dest := fmt.Sprintf("%s:%d", server, port)
	conn, err := grpc.Dial(dest, grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		log.Fatal(err)
	}
	client := hegel.NewHegelClient(conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = subscribe(ctx, client, func(str string) {
		fmt.Println(str)
	})
	if err != nil {
		log.Fatal(err)
	}
}

func subscribe(ctx context.Context, client hegel.HegelClient, onJSON func(string)) error {
	res, err := client.Get(ctx, &hegel.GetRequest{})
	if err != nil {
		return err
	}
	str := res.GetJSON()
	onJSON(str)

	watcher, err := client.Subscribe(ctx, &hegel.SubscribeRequest{})
	if err != nil {
		return err
	}

	for {
		hw, err := watcher.Recv()
		if errors.Is(err, io.EOF) {
			return errors.New("hegel closed the subscription")
		}

		onJSON(hw.GetJSON())
	}
}
