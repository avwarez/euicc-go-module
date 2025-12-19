package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"log/slog"

	"github.com/avwarez/euicc-go/driver/localnet"
	"github.com/damonto/euicc-go/driver/at"
	"github.com/damonto/euicc-go/driver/mbim"
	"github.com/damonto/euicc-go/driver/qmi"
	"github.com/damonto/euicc-go/lpa"
)

type Session struct {
	RemoteAddr     *net.UDPAddr
	LogicalChannel byte
	StartedAt      time.Time
	LastActivity   time.Time
}

var (
	channelMu      sync.RWMutex
	options        lpa.Options
	activeSession  *Session
	sessionTimeout = 60 * time.Second
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	bindAddrFlag := flag.String("bindAddr", "0.0.0.0", "Binding address")
	bindPortFlag := flag.Int("bindPort", 8080, "Binding port")
	bufferSizeFlag := flag.Int("bufferSize", 2048, "Buffer size in byte")
	timeoutFlag := flag.Int("timeout", 60, "Session timeout in seconds")
	flag.Parse()

	sessionTimeout = time.Duration(*timeoutFlag) * time.Second
	options.AdminProtocolVersion = "2"

	addr := net.UDPAddr{
		Port: *bindPortFlag,
		IP:   net.ParseIP(*bindAddrFlag),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		slog.Error("failed to start server", "error", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("shutdown signal received", "signal", sig)
		cancel()
		conn.Close()
	}()

	go sessionCleanup(ctx)

	slog.Info("server started", "address", addr.String(), "timeout", sessionTimeout)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down gracefully")
			cleanupActiveSession()
			return
		default:
		}

		buffer := make([]byte, *bufferSizeFlag)

		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {

				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
				slog.Error("error reading from socket", "error", err)
				continue
			}
		}

		pcRcv, err := localnet.Decode(buffer[:n])
		if err != nil {
			slog.Error("error decoding packet", "error", err)
			sendError(conn, remoteAddr, "invalid packet format")
			continue
		}

		slog.Debug("packet received", "packet", pcRcv, "from", remoteAddr)

		pcSnd := handleCommand(pcRcv, remoteAddr)

		if pcSnd == nil {
			pcSnd = localnet.NewPacketCmd(localnet.CmdResponse)
		}

		byteArrayResponse, err := localnet.Encode(pcSnd)
		if err != nil {
			slog.Error("error encoding response", "error", err)
			continue
		}

		_, err = conn.WriteToUDP(byteArrayResponse, remoteAddr)
		if err != nil {
			slog.Error("error sending response", "error", err)
			continue
		}

		slog.Debug("response sent", "to", remoteAddr)
	}
}

func handleCommand(pcRcv localnet.IPacketCmd, remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	switch pcRcv.GetCmd() {

	case localnet.CmdConnect:
		return handleConnect(pcRcv, remoteAddr)

	case localnet.CmdDisconnect:
		return handleDisconnect(remoteAddr)

	case localnet.CmdOpenLogical:
		return handleOpenLogical(pcRcv, remoteAddr)

	case localnet.CmdCloseLogical:
		return handleCloseLogical(pcRcv, remoteAddr)

	case localnet.CmdTransmit:
		return handleTransmit(pcRcv, remoteAddr)

	default:
		slog.Warn("unknown command", "command", pcRcv.GetCmd())
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "unknown command")
	}
}

func handleConnect(pcRcv localnet.IPacketCmd, remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	channelMu.Lock()
	defer channelMu.Unlock()

	if activeSession != nil {
		if time.Since(activeSession.LastActivity) < sessionTimeout {
			return localnet.NewPacketCmdErr(
				localnet.CmdResponse,
				fmt.Sprintf("device busy, in use by %s", activeSession.RemoteAddr),
			)
		}
		slog.Warn("forcing cleanup of expired session", "client", activeSession.RemoteAddr)
		forceCleanup()
	}

	pcConn, ok := pcRcv.(localnet.IPacketConnect)
	if !ok {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "invalid packet type for connect")
	}

	var err error
	switch pcConn.GetProto() {
	case "at":
		options.Channel, err = at.New(pcConn.GetDevice())
	case "mbim":
		options.Channel, err = mbim.New(pcConn.GetDevice(), pcConn.GetSlot())
	case "qmi":
		options.Channel, err = qmi.New(pcConn.GetDevice(), pcConn.GetSlot())
	case "qrtr":
		options.Channel, err = qmi.NewQRTR(pcConn.GetSlot())
	default:
		return localnet.NewPacketCmdErr(
			localnet.CmdResponse,
			fmt.Sprintf("unsupported protocol: %s", pcConn.GetProto()),
		)
	}

	if err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	err = options.Channel.Connect()
	if err != nil {
		options.Channel = nil
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	activeSession = &Session{
		RemoteAddr:     remoteAddr,
		LogicalChannel: localnet.InvalidChannel,
		StartedAt:      time.Now(),
		LastActivity:   time.Now(),
	}

	slog.Info("session started",
		"client", remoteAddr.String(),
		"protocol", pcConn.GetProto(),
		"device", pcConn.GetDevice())

	return localnet.NewPacketCmd(localnet.CmdResponse)
}

func handleDisconnect(remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	channelMu.Lock()
	defer channelMu.Unlock()

	if activeSession == nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "no active session")
	}

	if !addressesEqual(activeSession.RemoteAddr, remoteAddr) {
		return localnet.NewPacketCmdErr(
			localnet.CmdResponse,
			fmt.Sprintf("unauthorized: session belongs to %s", activeSession.RemoteAddr),
		)
	}

	if options.Channel != nil && activeSession.LogicalChannel != localnet.InvalidChannel {
		if err := options.Channel.CloseLogicalChannel(activeSession.LogicalChannel); err != nil {
			slog.Warn("failed to close logical channel", "error", err)
		}
	}

	var err error
	if options.Channel != nil {
		err = options.Channel.Disconnect()
		options.Channel = nil
	}

	slog.Info("session ended", "client", remoteAddr.String(), "duration", time.Since(activeSession.StartedAt))
	activeSession = nil

	if err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	return localnet.NewPacketCmd(localnet.CmdResponse)
}

func handleOpenLogical(pcRcv localnet.IPacketCmd, remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	channelMu.Lock()
	defer channelMu.Unlock()

	if err := checkSessionAuth(remoteAddr); err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	pktBody, ok := pcRcv.(localnet.IPacketBody)
	if !ok {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "invalid packet type")
	}

	aid := pktBody.GetBody()
	if len(aid) == 0 {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "empty AID")
	}

	channel, err := options.Channel.OpenLogicalChannel(aid)
	if err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	activeSession.LogicalChannel = channel
	activeSession.LastActivity = time.Now()

	slog.Debug("logical channel opened", "channel", channel, "aid", fmt.Sprintf("%X", aid))

	return localnet.NewPacketBody(localnet.CmdResponse, []byte{channel})
}

func handleCloseLogical(pcRcv localnet.IPacketCmd, remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	channelMu.Lock()
	defer channelMu.Unlock()

	if err := checkSessionAuth(remoteAddr); err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	pktBody, ok := pcRcv.(localnet.IPacketBody)
	if !ok || len(pktBody.GetBody()) == 0 {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "invalid packet")
	}

	channel := pktBody.GetBody()[0]

	err := options.Channel.CloseLogicalChannel(channel)
	if err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	if activeSession.LogicalChannel == channel {
		activeSession.LogicalChannel = localnet.InvalidChannel
	}
	activeSession.LastActivity = time.Now()

	slog.Debug("logical channel closed", "channel", channel)

	return localnet.NewPacketCmd(localnet.CmdResponse)
}

func handleTransmit(pcRcv localnet.IPacketCmd, remoteAddr *net.UDPAddr) localnet.IPacketCmd {
	channelMu.Lock()
	defer channelMu.Unlock()

	if err := checkSessionAuth(remoteAddr); err != nil {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	pktBody, ok := pcRcv.(localnet.IPacketBody)
	if !ok {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "invalid packet type")
	}

	apdu := pktBody.GetBody()
	if len(apdu) == 0 {
		return localnet.NewPacketCmdErr(localnet.CmdResponse, "empty APDU")
	}

	response, err := options.Channel.Transmit(apdu)
	if err != nil {
		slog.Error("transmit failed", "error", err)
		return localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
	}

	activeSession.LastActivity = time.Now()

	slog.Debug("transmit completed",
		"apduLen", len(apdu),
		"responseLen", len(response))

	return localnet.NewPacketBody(localnet.CmdResponse, response)
}

func checkSessionAuth(remoteAddr *net.UDPAddr) error {
	if activeSession == nil {
		return fmt.Errorf("no active session, connect first")
	}

	if !addressesEqual(activeSession.RemoteAddr, remoteAddr) {
		return fmt.Errorf("unauthorized: session belongs to %s", activeSession.RemoteAddr)
	}

	if time.Since(activeSession.LastActivity) > sessionTimeout {
		slog.Warn("session expired during operation")
		forceCleanup()
		return fmt.Errorf("session expired")
	}

	return nil
}

func sessionCleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			channelMu.Lock()
			if activeSession != nil && time.Since(activeSession.LastActivity) > sessionTimeout {
				slog.Info("cleaning up expired session",
					"client", activeSession.RemoteAddr,
					"idleTime", time.Since(activeSession.LastActivity))
				forceCleanup()
			}
			channelMu.Unlock()
		}
	}
}

func forceCleanup() {
	if activeSession != nil && options.Channel != nil {

		if activeSession.LogicalChannel != localnet.InvalidChannel {
			options.Channel.CloseLogicalChannel(activeSession.LogicalChannel)
		}
		options.Channel.Disconnect()
		options.Channel = nil
	}
	activeSession = nil
}

func cleanupActiveSession() {
	channelMu.Lock()
	defer channelMu.Unlock()
	forceCleanup()
}

func addressesEqual(a1, a2 *net.UDPAddr) bool {
	if a1 == nil || a2 == nil {
		return false
	}
	return a1.IP.Equal(a2.IP) && a1.Port == a2.Port
}

func sendError(conn *net.UDPConn, addr *net.UDPAddr, errMsg string) {
	pcErr := localnet.NewPacketCmdErr(localnet.CmdResponse, errMsg)
	if data, err := localnet.Encode(pcErr); err == nil {
		conn.WriteToUDP(data, addr)
	}
}
