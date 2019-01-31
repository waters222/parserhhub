package main

import (
	"flag"
	"fmt"
	"github.com/weishi258/parserhhub/log"
	"github.com/weishi258/parserhhub/proxy"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sigChan := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	resetServerChan := make(chan bool, 1)
	signal.Notify(sigChan,
		syscall.SIGTERM,
		syscall.SIGINT)

	var localPort int
	var logFile string
	var logLevel string
	flag.StringVar(&logFile, "log", "", "log output file path")
	flag.StringVar(&logLevel, "l", "info", "log level")
	flag.IntVar(&localPort, "port", 8000, "server listening port")
	flag.Parse()

	var err error
	defer func() {
		if err != nil {
			os.Exit(1)
		} else {
			os.Exit(0)
		}
	}()

	logger := log.InitLogger(logFile, logLevel, false)
	server := proxy.NewProxyServer(fmt.Sprintf("0.0.0.0:%d", localPort))
	if err = server.Start(resetServerChan, false); err != nil {
		log.GetLogger().Fatal("start proxy server failed", zap.String("error", err.Error()))
		return
	}

	logger.Info("proxy server start successful")
	go func() {
		select {
		case sig := <-sigChan:
			logger.Debug("caught signal for exit", zap.Any("signal", sig))

			done <- true
		case <-resetServerChan:
			logger.Fatal("proxy server crashed, quiting")
			done <- true
		}

	}()
	<-done
	logger.Info("proxy quited")
}
