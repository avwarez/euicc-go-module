package main

import (
	"flag"
	"fmt"
	"net"
	"sync"

	"log/slog"

	"github.com/avwarez/euicc-go/driver/localnet"
	"github.com/damonto/euicc-go/driver/at"
	"github.com/damonto/euicc-go/driver/mbim"
	"github.com/damonto/euicc-go/driver/qmi"
	"github.com/damonto/euicc-go/lpa"
)

var (
	channelMu sync.RWMutex
	options   lpa.Options
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	bindAddrFlag := flag.String("bindAddr", "0.0.0.0", "Binding address")
	bindPortFlag := flag.Int("bindPort", 8080, "Binding port")
	bufferSizeFlag := flag.Int("bufferSize", 2048, "Buffer size in byte")
	flag.Parse()

	options.AdminProtocolVersion = "2"

	addr := net.UDPAddr{
		Port: *bindPortFlag,
		IP:   net.ParseIP(*bindAddrFlag),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		slog.Error("Error on socket server listening", "error", err)
		return
	}
	defer conn.Close()

outer:
	for {
		buffer := make([]byte, *bufferSizeFlag)

		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			slog.Error("error reading from socket", "error", err)
			break
		}

		var pcRcv, errr = localnet.Decode(buffer[:n])
		if errr != nil {
			slog.Error("Error decoding packet. Closing server")
			break
		}

		slog.Debug("Read f sock", "packet", pcRcv)

		var pcSnd localnet.IPacketCmd = nil

		switch pcRcv.GetCmd() {

		case localnet.CmdConnect:

			channelMu.Lock()
			if options.Channel != nil {
				err = fmt.Errorf("error: channel already open, retry later")
			} else {
				var pcConn localnet.IPacketConnect = pcRcv.(localnet.IPacketConnect)

				switch pcConn.GetProto() {
				case "at":
					options.Channel, err = at.New(pcConn.GetDevice())
				/*case "ccid":
				options.Channel, err = ccid.New() */
				case "mbim":
					options.Channel, err = mbim.New(pcConn.GetDevice(), pcConn.GetSlot())
				case "qmi":
					options.Channel, err = qmi.New(pcConn.GetDevice(), pcConn.GetSlot())
				case "qrtr":
					options.Channel, err = qmi.NewQRTR(pcConn.GetSlot())
				default:
					err = fmt.Errorf("error: no handler for the specified protocol %s", pcConn.GetProto())
				}
			}
			channelMu.Unlock()

			if err != nil {
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			} else {

				err = options.Channel.Connect()
				if err != nil {
					pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
					options.Channel = nil
				}
			}

		case localnet.CmdDisconnect:
			err = options.Channel.Disconnect()
			options.Channel = nil
			if err != nil {
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			}

		case localnet.CmdOpenLogical:
			var channel byte
			channel, err = options.Channel.OpenLogicalChannel(pcRcv.(localnet.IPacketBody).GetBody())
			var bb = []byte{channel}
			if err != nil {
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			} else {
				pcSnd = localnet.NewPacketBody(localnet.CmdResponse, bb)
			}

		case localnet.CmdCloseLogical:
			err = options.Channel.CloseLogicalChannel(pcRcv.(localnet.IPacketBody).GetBody()[0])
			if err != nil {
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			}

		case localnet.CmdTransmit:
			var bb, err = options.Channel.Transmit(pcRcv.(localnet.IPacketBody).GetBody())
			if err != nil {
				slog.Error("Error on transmit", "error", err)
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			} else {
				pcSnd = localnet.NewPacketBody(localnet.CmdResponse, bb)
			}
			slog.Debug("Send t sock", "packet", pcSnd)

		default:
			slog.Error("Receiving unknown command. Closing server")
			break outer
		}

		if pcSnd == nil {
			pcSnd = localnet.NewPacketCmd(localnet.CmdResponse)
		}
		byteArrayResponse, err := localnet.Encode(pcSnd)
		if err != nil {
			slog.Error("Error encoding response", "error", err)
			break
		}

		_, err = conn.WriteToUDP(byteArrayResponse, remoteAddr)
		if err != nil {
			slog.Error("Error sending response to the client", "error", err)
			break
		}

	}
}
