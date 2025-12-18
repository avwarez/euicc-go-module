package main

import (
	"flag"
	"fmt"
	"net"

	"log/slog"

	"github.com/avwarez/euicc-go/driver/localnet"
	"github.com/damonto/euicc-go/driver/at"
	"github.com/damonto/euicc-go/driver/mbim"
	"github.com/damonto/euicc-go/driver/qmi"
	"github.com/damonto/euicc-go/lpa"
)

var (
	options lpa.Options
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
		fmt.Println("Error on socket server listening:", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, *bufferSizeFlag)

outer:
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Printf("error reading from socket %s\n", err)
			break
		}

		var pcRcv, errr = localnet.Decode(buffer[:n])
		if errr != nil {
			fmt.Printf("Error decoding packet. Closing server\n")
			break
		}

		fmt.Printf("DEBUG %s\n", pcRcv)

		var pcSnd localnet.IPacketCmd = nil

		switch pcRcv.GetCmd() {

		case localnet.CmdConnect:

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
				fmt.Printf("Error on transmit: %s\n", err)
				pcSnd = localnet.NewPacketCmdErr(localnet.CmdResponse, err.Error())
			} else {
				pcSnd = localnet.NewPacketBody(localnet.CmdResponse, bb)
			}
			fmt.Printf("DEBUG %s\n", pcSnd)

		default:
			fmt.Printf("Receiving unknown command. Closing server\n")
			break outer
		}

		if pcSnd == nil {
			pcSnd = localnet.NewPacketCmd(localnet.CmdResponse)
		}
		byteArrayResponse, err := localnet.Encode(pcSnd)
		if err != nil {
			fmt.Printf("Error encoding response: %s\n", err)
			break
		}

		_, err = conn.WriteToUDP(byteArrayResponse, remoteAddr)
		if err != nil {
			fmt.Printf("Error sending response to the client: %s\n", err)
			break
		}

	}
}
