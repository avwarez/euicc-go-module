package localnet

import (
	"errors"
	"fmt"
	"net"

	"github.com/damonto/euicc-go/apdu"
)

type NetContext struct {
	serverAddr string
	rAddr      *net.UDPAddr
	conn       *net.UDPConn
	device     string
	proto      string
	slot       uint8
	bufferSize uint16
}

type NetConf struct {
}

func NewUDP(serverAddr string, device string, proto string, slot uint8, bufferSize uint16) (apdu.SmartCardChannel, error) {
	rAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("error resolving address: %s %w", serverAddr, err)
	}

	netctx := &NetContext{serverAddr: serverAddr, rAddr: rAddr, device: device, proto: proto, slot: slot, bufferSize: bufferSize}
	return netctx, nil
}

func (c *NetContext) Connect() error {
	conn, err := net.DialUDP("udp", nil, c.rAddr)
	if err != nil {
		return fmt.Errorf("error establishing connection with %s %w", c.rAddr, err)
	}
	c.conn = conn

	_, err = remoteCall(c, NewPacketConnect(c.device, c.proto, c.slot))
	return err
}

func (c *NetContext) Disconnect() error {
	var err error
	if c.conn != nil {
		_, err = remoteCall(c, NewPacketCmd(CmdDisconnect))
		c.conn.Close()
		c.conn = nil
	}
	return err
}

func (c *NetContext) Transmit(command []byte) ([]byte, error) {
	return remoteCall(c, NewPacketBody(CmdTransmit, command))
}

func (c *NetContext) OpenLogicalChannel(AID []byte) (byte, error) {
	bb, er := remoteCall(c, NewPacketBody(CmdOpenLogical, AID))
	if er != nil {
		return 255, er
	} else if bb == nil || len(bb) != 1 {
		return 255, errors.New("openlogicalchannel: empty channel received")
	}
	return bb[0], er
}

func (c *NetContext) CloseLogicalChannel(channel byte) error {
	_, er := remoteCall(c, NewPacketBody(CmdCloseLogical, []byte{channel}))
	return er
}

func remoteCall(nc *NetContext, pcSnd IPacketCmd) (by []byte, er error) {

	byteToTransmit, err1 := Encode(pcSnd)
	if err1 != nil {
		return nil, fmt.Errorf("error encoding message %s %w", pcSnd, err1)
	}

	_, err2 := nc.conn.Write(byteToTransmit)
	if err2 != nil {
		return nil, fmt.Errorf("error sending message %s %w", pcSnd, err2)
	}

	if nc.bufferSize <= 0 {
		nc.bufferSize = 2048
	}
	buffer := make([]byte, nc.bufferSize)
	n, _, err3 := nc.conn.ReadFromUDP(buffer)
	if err3 != nil {
		return nil, fmt.Errorf("error receiving response %X %w", buffer, err3)
	}

	pcRcv, err4 := Decode(buffer[:n])
	if err4 != nil {
		return nil, fmt.Errorf("error decoding response %X %w", buffer[:n], err4)
	}

	if pcRcv.GetErr() != "" {
		return nil, fmt.Errorf("error on server %s", pcRcv.GetErr())
	}

	if ext, ok := pcRcv.(IPacketBody); ok {
		return ext.GetBody(), nil
	}
	return nil, nil
}
