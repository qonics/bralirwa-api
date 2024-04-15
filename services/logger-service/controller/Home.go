package controller

import (
	"context"
	"libs/shared-package/proto"
	"os"

	"github.com/phuslu/log"
)

type Server struct {
	proto.UnimplementedLoggerServiceServer
}

func Hello() string {
	return "Logger service is running..."
}

func (g *Server) Log(ctx context.Context, req *proto.LogRequest) (*proto.SuccessResponse, error) {
	// fmt.Println(req.Message, req.LogTime, req.ServiceName, req.Identifier, req.LogLevel)
	// config := config.InitializeConfig()
	var logger log.Logger
	if log.IsTerminal(os.Stderr.Fd()) {
		logger = log.Logger{
			Level:  log.ParseLevel(req.LogLevel),
			Caller: 1,
			Writer: &log.ConsoleWriter{
				ColorOutput:    true,
				EndWithMessage: true,
			},
		}
	} else {
		logger = log.Logger{
			Level:      log.ParseLevel(req.LogLevel),
			Caller:     0,
			TimeField:  "",
			TimeFormat: "",
			Writer: &log.FileWriter{
				Filename: "logs/log.log",
				MaxSize:  0,
				// MaxBackups:   config.Log.Backups,
				LocalTime:    true,
				FileMode:     os.FileMode(0600),
				EnsureFolder: true,
			},
		}
	}
	logger.Log().Str("Service", req.ServiceName).Str("Identifier", req.Identifier).Msg(req.Message)
	return &proto.SuccessResponse{Response: "success"}, nil
}
