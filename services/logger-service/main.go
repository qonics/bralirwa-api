package main

import (
	"fmt"
	"logger-service/config"
	"logger-service/controller"
	"net"

	"libs/shared-package/proto"

	"github.com/phuslu/log"
	"google.golang.org/grpc"
)

func main() {
	config := config.InitializeConfig()
	fmt.Println("Logger-service: initialization", config.Port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.Fatal().Err(err).Int("Port", config.Port).Msg("failed to listen")
	}

	grpcServer := grpc.NewServer()
	proto.RegisterLoggerServiceServer(grpcServer, &controller.Server{})
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal().Err(err).Msg("failed to serve")
	}
}
